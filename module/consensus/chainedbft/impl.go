/*
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chainedbft

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"chainmaker.org/chainmaker-go/chainconf"
	"chainmaker.org/chainmaker-go/consensus/chainedbft/message"
	timeservice "chainmaker.org/chainmaker-go/consensus/chainedbft/time_service"
	"chainmaker.org/chainmaker-go/consensus/chainedbft/utils"
	"chainmaker.org/chainmaker-go/consensus/governance"
	"chainmaker.org/chainmaker-go/localconf"
	"chainmaker.org/chainmaker-go/logger"
	"chainmaker.org/chainmaker/common/msgbus"
	"chainmaker.org/chainmaker/pb-go/common"
	"chainmaker.org/chainmaker/pb-go/consensus"
	chainedbftpb "chainmaker.org/chainmaker/pb-go/consensus/chainedbft"
	"chainmaker.org/chainmaker/pb-go/net"
	"chainmaker.org/chainmaker/protocol"

	"github.com/gogo/protobuf/proto"
	"github.com/tidwall/wal"
)

const (
	CONSENSUSCAPABILITY = 100000
	INTERNALCAPABILITY  = 100000
	ModuleName          = "chainedbft"
	WalDirSuffix        = "hotstuff_wal"
)

// ConsensusChainedBftImpl implements chained hotstuff consensus protocol
type ConsensusChainedBftImpl struct {
	id               string // The identity of the local node
	chainID          string // chain ID
	selfIndexInEpoch uint64 // Index of the local node in the validator collection of the current epoch

	msgCh           chan *net.NetMsg                // Receive information from the msgBus
	consBlockCh     chan *common.Block              // Transmit the committed block information
	proposedBlockCh chan *common.Block              // Transmit the block information generated by the local node
	syncReqMsgCh    chan *chainedbftpb.ConsensusMsg // Transmit request information with the block
	syncRespMsgCh   chan *chainedbftpb.ConsensusMsg // Transmit response information with the block
	internalMsgCh   chan *chainedbftpb.ConsensusMsg // Transmit the own proposals, voting information by the local node
	protocolMsgCh   chan *chainedbftpb.ConsensusMsg // Transmit Hotstuff protocol information: proposal, vote

	//nextEpoch          *epochManager       // next epoch
	commitHeight       uint64              // The height of the latest committed block
	government         protocol.Government // The management contract on the block chain
	lastCommitWalIndex uint64

	// wal info
	wal              *wal.Log
	proposalWalIndex sync.Map
	doneReplayWal    bool

	// Services within the module
	smr          *chainedbftSMR            // State machine replication in hotstuff
	syncer       *syncManager              // The information synchronization of the consensus module
	msgPool      *message.MsgPool          // manages all of consensus messages received for protocol
	chainStore   *chainStore               // Cache blocks, status information of QC, and the process of the commit blocks on the chain
	timerService *timeservice.TimerService // Timer service

	// Services of other modules
	logger                *logger.CMLogger
	msgbus                msgbus.MessageBus
	singer                protocol.SigningMember
	helper                protocol.HotStuffHelper
	store                 protocol.BlockchainStore
	chainConf             protocol.ChainConf
	netService            protocol.NetService
	ledgerCache           protocol.LedgerCache
	blockVerifier         protocol.BlockVerifier
	blockCommitter        protocol.BlockCommitter
	proposalCache         protocol.ProposalCache
	accessControlProvider protocol.AccessControlProvider

	// Exit signal
	quitCh        chan struct{}
	quitSyncReqCh chan struct{}
}

//New returns an instance of chainedbft consensus
func New(chainID string, id string, singer protocol.SigningMember, ac protocol.AccessControlProvider,
	ledgerCache protocol.LedgerCache, proposalCache protocol.ProposalCache, blockVerifier protocol.BlockVerifier,
	blockCommitter protocol.BlockCommitter, netService protocol.NetService, store protocol.BlockchainStore,
	msgBus msgbus.MessageBus, chainConf protocol.ChainConf, helper protocol.HotStuffHelper) (*ConsensusChainedBftImpl, error) {

	service := &ConsensusChainedBftImpl{
		id:                 id,
		chainID:            chainID,
		msgCh:              make(chan *net.NetMsg, CONSENSUSCAPABILITY),
		syncReqMsgCh:       make(chan *chainedbftpb.ConsensusMsg, INTERNALCAPABILITY),
		syncRespMsgCh:      make(chan *chainedbftpb.ConsensusMsg, INTERNALCAPABILITY),
		internalMsgCh:      make(chan *chainedbftpb.ConsensusMsg, INTERNALCAPABILITY),
		protocolMsgCh:      make(chan *chainedbftpb.ConsensusMsg, INTERNALCAPABILITY),
		consBlockCh:        make(chan *common.Block, INTERNALCAPABILITY),
		proposedBlockCh:    make(chan *common.Block, INTERNALCAPABILITY),
		proposalWalIndex:   sync.Map{},
		lastCommitWalIndex: 1,

		store:                 store,
		singer:                singer,
		helper:                helper,
		msgbus:                msgBus,
		chainConf:             chainConf,
		netService:            netService,
		ledgerCache:           ledgerCache,
		proposalCache:         proposalCache,
		blockVerifier:         blockVerifier,
		blockCommitter:        blockCommitter,
		accessControlProvider: ac,
		logger:                logger.GetLoggerByChain(logger.MODULE_CONSENSUS, chainConf.ChainConfig().ChainId),
		government:            governance.NewGovernanceContract(store, ledgerCache),

		quitCh:        make(chan struct{}),
		quitSyncReqCh: make(chan struct{}),
	}
	lastCommitBlk := ledgerCache.GetLastCommittedBlock()
	if lastCommitBlk == nil {
		return nil, fmt.Errorf("openChainStore failed, get best block from ledger")
	}
	service.syncer = newSyncManager(service)
	service.commitHeight = lastCommitBlk.Header.BlockHeight
	service.timerService = timeservice.NewTimerService(service.logger)

	var err error
	if service.chainStore, err = initChainStore(service); err != nil {
		service.logger.Errorf("new consensus service failed, err %v", err)
		return nil, err
	}

	var epoch *epochManager
	if epoch, err = service.createEpoch(service.commitHeight); err != nil {
		return nil, err
	}
	service.msgPool = epoch.msgPool
	service.selfIndexInEpoch = epoch.index
	if service.smr, err = newChainedBftSMR(service, epoch); err != nil {
		return nil, err
	}
	walDirPath := path.Join(localconf.ChainMakerConfig.StorageConfig.StorePath, chainID, WalDirSuffix)
	if service.wal, err = wal.Open(walDirPath, nil); err != nil {
		return nil, err
	}
	service.logger.Debugf("init epoch, epochID: %d, index: %d, createHeight: %d", epoch.epochId, epoch.index, epoch.createHeight)
	if err := chainconf.RegisterVerifier(chainID, consensus.ConsensusType_HOTSTUFF, service.government); err != nil {
		return nil, err
	}
	service.initTimeOutConfig(service.government)
	return service, nil
}

func (cbi *ConsensusChainedBftImpl) initTimeOutConfig(government protocol.Government) {
	contract, _ := government.GetGovernanceContract()
	base := contract.GetHotstuffRoundTimeoutMill()
	if base == 0 {
		base = uint64(timeservice.DefaultRoundTimeout)
	}
	if err := utils.VerifyTimeConfig(governance.RoundTimeoutMill, base); err == nil {
		timeservice.RoundTimeout = time.Duration(base) * time.Millisecond
	}

	delta := contract.GetHotstuffRoundTimeoutIntervalMill()
	if delta == 0 {
		delta = uint64(timeservice.DefaultRoundTimeoutInterval)
	}
	if err := utils.VerifyTimeConfig(governance.RoundTimeoutIntervalMill, delta); err == nil {
		timeservice.RoundTimeoutInterval = time.Duration(delta) * time.Millisecond
	}
}

//Start start consensus
func (cbi *ConsensusChainedBftImpl) Start() error {
	cbi.logger.Infof("consensus.chainedBft service started")
	cbi.msgbus.Register(msgbus.ProposedBlock, cbi)
	cbi.msgbus.Register(msgbus.RecvConsensusMsg, cbi)
	cbi.msgbus.Register(msgbus.BlockInfo, cbi)
	cbi.logger.Debugf("add config watch begin...")
	//cbi.chainConf.AddWatch(cbi)
	cbi.logger.Debugf("end config watch begin...")

	go cbi.syncer.start()
	go cbi.timerService.Start()
	go cbi.loop()
	go cbi.syncReqLoop()
	cbi.startConsensus()
	return nil
}

func (cbi *ConsensusChainedBftImpl) startConsensus() {
	hasWalEntry := cbi.replayWal()
	if hasWalEntry {
		return
	}
	highQC := cbi.chainStore.getCurrentQC()
	cbi.processCertificates(highQC, nil)
	if cbi.isValidProposer(cbi.smr.getHeight(), cbi.smr.getCurrentLevel(), cbi.selfIndexInEpoch) {
		cbi.smr.updateState(chainedbftpb.ConsStateType_PROPOSE)
		cbi.processNewPropose(cbi.smr.getHeight(), cbi.smr.getCurrentLevel(), cbi.chainStore.getCurrentQC().BlockId)
	}
}

//Stop stop consensus
func (cbi *ConsensusChainedBftImpl) Stop() error {
	close(cbi.quitCh)
	close(cbi.quitSyncReqCh)
	if cbi.timerService != nil {
		cbi.timerService.Stop()
	}
	if cbi.msgPool != nil {
		cbi.msgPool.Cleanup()
	}
	return nil
}

//OnMessage MsgBus implement interface, receive message from MsgBus
func (cbi *ConsensusChainedBftImpl) OnMessage(message *msgbus.Message) {
	cbi.logger.Debugf("id [%s] OnMessage receive topic: %s", cbi.id, message.Topic)
	switch message.Topic {
	case msgbus.ProposedBlock:
		if proposedBlock, ok := message.Payload.(*consensus.ProposalBlock); ok {
			cbi.proposedBlockCh <- proposedBlock.Block
		}
	case msgbus.RecvConsensusMsg:
		if netMsg, ok := message.Payload.(*net.NetMsg); ok {
			cbi.msgCh <- netMsg
		}
	case msgbus.BlockInfo:
		if blockInfo, ok := message.Payload.(*common.BlockInfo); ok {
			if blockInfo == nil || blockInfo.Block == nil {
				cbi.logger.Errorf("error message BlockInfo is nil")
				return
			}
			cbi.consBlockCh <- blockInfo.Block
		}
	}
}

func (cbi *ConsensusChainedBftImpl) loop() {
	for {
		select {
		case msg, ok := <-cbi.msgCh:
			if ok {
				cbi.onReceivedMsg(msg)
			}
		case msg, ok := <-cbi.internalMsgCh:
			if ok {
				cbi.onConsensusMsg(msg)
			}
		case msg, ok := <-cbi.proposedBlockCh:
			if ok {
				cbi.onProposedBlock(msg)
			}
		case block, ok := <-cbi.consBlockCh:
			if ok {
				cbi.onBlockCommitted(block)
			}
		case firedEvent, ok := <-cbi.timerService.GetFiredCh():
			if ok {
				cbi.onFiredEvent(firedEvent)
			}
		case msg, ok := <-cbi.protocolMsgCh:
			if ok {
				switch msg.Payload.Type {
				case chainedbftpb.MessageType_PROPOSAL_MESSAGE:
					cbi.onReceivedProposal(msg)
				case chainedbftpb.MessageType_VOTE_MESSAGE:
					cbi.onReceivedVote(msg)
				default:
					cbi.logger.Warnf("service selfIndexInEpoch [%v] received non-protocol msg %v", cbi.selfIndexInEpoch, msg.Payload.Type)
				}
			}
		case msg, ok := <-cbi.syncRespMsgCh:
			if ok {
				cbi.onReceiveBlockFetchRsp(msg)
			}
		case <-cbi.quitCh:
			return
		}
	}
}

func (cbi *ConsensusChainedBftImpl) syncReqLoop() {
	for {
		select {
		case msg, ok:= <-cbi.syncReqMsgCh:
			if ok {
				cbi.onReceiveBlockFetch(msg)
			}
		case <-cbi.quitSyncReqCh:
			return
		}
	}
}

//OnQuit msgbus quit
func (cbi *ConsensusChainedBftImpl) OnQuit() {
	// do nothing
}

//Module chainedBft
func (cbi *ConsensusChainedBftImpl) Module() string {
	return ModuleName
}

func (cbi *ConsensusChainedBftImpl) onReceivedMsg(msg *net.NetMsg) {
	if msg == nil {
		cbi.logger.Warnf("service selfIndexInEpoch [%v] received nil message", cbi.selfIndexInEpoch)
		return
	}
	if msg.Type != net.NetMsg_CONSENSUS_MSG {
		cbi.logger.Warnf("service selfIndexInEpoch [%v] received unsubscribed msg %v to %v",
			cbi.selfIndexInEpoch, msg.Type, msg.To)
		return
	}

	cbi.logger.Debugf("service selfIndexInEpoch [%v] received a consensus msg from remote peer "+
		"id %v addr %v", cbi.selfIndexInEpoch, msg.Type, msg.To)
	consensusMsg := new(chainedbftpb.ConsensusMsg)
	if err := proto.Unmarshal(msg.Payload, consensusMsg); err != nil {
		cbi.logger.Errorf("service selfIndexInEpoch [%v] failed to unmarshal consensus data %v, err %v",
			cbi.selfIndexInEpoch, msg.Payload, err)
		return
	}
	if consensusMsg.Payload == nil {
		cbi.logger.Errorf("service selfIndexInEpoch [%v] received invalid consensus msg with nil payload "+
			"from remote peer id [%v] add %v", cbi.selfIndexInEpoch, msg.Type, msg.To)
		return
	}
	if err := message.ValidateMessageBasicInfo(consensusMsg.Payload); err != nil {
		cbi.logger.Errorf("service selfIndexInEpoch [%v] failed to validate msg basic info, err %v",
			cbi.selfIndexInEpoch, err)
		return
	}
	cbi.onConsensusMsg(consensusMsg)
}

//onConsensusMsg dispatches consensus msg to handler
func (cbi *ConsensusChainedBftImpl) onConsensusMsg(msg *chainedbftpb.ConsensusMsg) {
	cbi.logger.Debugf("service selfIndexInEpoch [%v] dispatch msg %v to related channel",
		cbi.selfIndexInEpoch, msg.Payload.Type)
	t := time.NewTimer(timeservice.RoundTimeout)
	defer t.Stop()

	switch msg.Payload.Type {
	case chainedbftpb.MessageType_PROPOSAL_MESSAGE:
		select {
		case cbi.protocolMsgCh <- msg:
		case <-t.C:
		}
	case chainedbftpb.MessageType_VOTE_MESSAGE:
		select {
		case cbi.protocolMsgCh <- msg:
		case <-t.C:
		}
	case chainedbftpb.MessageType_BLOCK_FETCH_MESSAGE:
		select {
		case cbi.syncReqMsgCh <- msg:
		case <-t.C:
		}
	case chainedbftpb.MessageType_BLOCK_FETCH_RESP_MESSAGE:
		select {
		case cbi.syncRespMsgCh <- msg:
		case <-t.C:
		}
	}
}

//onFiredEvent dispatches timer event to handler
func (cbi *ConsensusChainedBftImpl) onFiredEvent(te *timeservice.TimerEvent) {
	if te.Level < cbi.smr.getCurrentLevel() || te.EpochId != cbi.smr.getEpochId() {
		cbi.logger.Debugf("service selfIndexInEpoch [%v] onFiredEvent: fired event %v, smr:"+
			" height [%v], level [%v], state [%v], epoch [%v]", cbi.selfIndexInEpoch, te,
			cbi.smr.getHeight(), cbi.smr.getCurrentLevel(), cbi.smr.state, cbi.smr.getEpochId())
		return
	}
	cbi.logger.Infof("receive time out event, state: %s, height: %d, level: %d, duration: %s", te.State.String(), te.Height, te.Level, te.Duration.String())
	switch te.State {
	case chainedbftpb.ConsStateType_PACEMAKER:
		cbi.processLocalTimeout(te.Height, te.Level)
	default:
		cbi.logger.Errorf("service selfIndexInEpoch [%v] received invalid event %v", cbi.selfIndexInEpoch, te)
	}
}

//onReceiveBlockFetch handles a block fetch request
func (cbi *ConsensusChainedBftImpl) onReceiveBlockFetch(msg *chainedbftpb.ConsensusMsg) {
	cbi.processBlockFetch(msg)
}

//onReceiveBlockFetchRsp handles a block fetch response
func (cbi *ConsensusChainedBftImpl) onReceiveBlockFetchRsp(msg *chainedbftpb.ConsensusMsg) {
	cbi.processFetchResp(msg)
}

//onBlockCommitted update the consensus smr to latest
func (cbi *ConsensusChainedBftImpl) onBlockCommitted(block *common.Block) {
	cbi.processBlockCommitted(block)
}

//onProposedBlock
func (cbi *ConsensusChainedBftImpl) onProposedBlock(block *common.Block) {
	cbi.processProposedBlock(block)
}

func (cbi *ConsensusChainedBftImpl) onReceivedVote(msg *chainedbftpb.ConsensusMsg) {
	cbi.processVote(msg)
}

func (cbi *ConsensusChainedBftImpl) onReceivedProposal(msg *chainedbftpb.ConsensusMsg) {
	cbi.processProposal(msg)
}

func (cbi *ConsensusChainedBftImpl) onReceivedCommit(msg interface{}) {
	cbi.processCommit(msg)
}

// VerifyBlockSignatures verify consensus qc at incoming block
func (cbi *ConsensusChainedBftImpl) VerifyBlockSignatures(block *common.Block) error {
	if block == nil || block.AdditionalData == nil ||
		len(block.AdditionalData.ExtraData) <= 0 {
		return errors.New("nil block or nil additionalData or empty extraData")
	}

	var (
		err           error
		quorumCert    []byte
		newViewNum    int
		votedBlockNum int
		BlockId       = block.GetHeader().GetBlockHash()
	)
	if quorumCert = utils.GetQCFromBlock(block); len(quorumCert) == 0 {
		return errors.New("qc is nil")
	}
	qc := new(chainedbftpb.QuorumCert)
	if err = proto.Unmarshal(quorumCert, qc); err != nil {
		cbi.logger.Errorf("service selfIndexInEpoch [%v] unmarshal qc failed, err %v", cbi.selfIndexInEpoch, err)
		return fmt.Errorf("unmarshal qc failed, err %v", err)
	}
	if qc.BlockId == nil {
		cbi.logger.Errorf("service selfIndexInEpoch [%v] validate qc failed, nil block id", cbi.selfIndexInEpoch)
		return fmt.Errorf("nil block id in qc")
	}
	if !bytes.Equal(qc.BlockId, BlockId) {
		cbi.logger.Errorf("service selfIndexInEpoch [%v] validate qc failed, wrong qc BlockId [%v],"+
			"expected [%v]", cbi.selfIndexInEpoch, qc.BlockId, BlockId)
		return fmt.Errorf("wrong qc block id [%v], expected [%v]",
			qc.BlockId, BlockId)
	}
	if newViewNum, votedBlockNum, err = cbi.countNumFromVotes(qc); err != nil {
		return err
	}
	quorum := cbi.smr.min(qc.Height)
	if qc.Level > 0 && qc.NewView && newViewNum < quorum {
		return fmt.Errorf(fmt.Sprintf("vote new view num [%v] less than expected [%v]",
			newViewNum, quorum))
	}
	if qc.Level > 0 && !qc.NewView && votedBlockNum < quorum {
		return fmt.Errorf(fmt.Sprintf("vote block num [%v] less than expected [%v]",
			votedBlockNum, quorum))
	}
	return nil
}

func (cbi *ConsensusChainedBftImpl) countNumFromVotes(qc *chainedbftpb.QuorumCert) (int, int, error) {
	var (
		votedBlock   = make(map[uint64]*chainedbftpb.VoteData)
		votedNewView = make(map[uint64]*chainedbftpb.VoteData)
		voteIdxs     = make(map[uint64]bool, 0)
	)
	//for each vote
	for _, vote := range qc.Votes {
		if vote == nil {
			return 0, 0, fmt.Errorf("vote is nil")
		}
		if err := cbi.validateVoteData(vote); err != nil {
			return 0, 0, fmt.Errorf("invalid commits, err %v", err)
		}
		if vote.Height != qc.Height || vote.Level != qc.Level {
			return 0, 0, fmt.Errorf("vote for wrong height:round:level [%v:%v], expected [%v:%v]",
				vote.Height, vote.Level, qc.Height, qc.Level)
		}
		if ok := voteIdxs[vote.AuthorIdx]; ok {
			return 0, 0, fmt.Errorf("duplicate vote index [%v] at height:round:level [%v:%v]",
				vote.AuthorIdx, vote.Height, vote.Level)
		}
		voteIdxs[vote.AuthorIdx] = true
		if vote.NewView {
			votedNewView[vote.AuthorIdx] = vote
		}
		if len(vote.BlockId) > 0 && bytes.Equal(vote.BlockId, qc.BlockId) {
			votedBlock[vote.AuthorIdx] = vote
		}
	}
	return len(votedNewView), len(votedBlock), nil
}

//VerifyBlockSignatures verify consensus qc at incoming block and chainconf
//now, implement check commit in all nodes, not in selected committee
func VerifyBlockSignatures(chainConf protocol.ChainConf, ac protocol.AccessControlProvider,
	store protocol.BlockchainStore, block *common.Block, ledger protocol.LedgerCache) error {
	// 1. base info check
	if block == nil || block.AdditionalData == nil || len(block.AdditionalData.ExtraData) <= 0 {
		return errors.New("nil block or nil additionalData or empty extraData")
	}
	qc := new(chainedbftpb.QuorumCert)
	if quorumCert := utils.GetQCFromBlock(block); len(quorumCert) == 0 {
		return errors.New("nil qc")
	} else {
		if err := proto.Unmarshal(quorumCert, qc); err != nil {
			return fmt.Errorf("failed to unmarshal qc, err %v", err)
		}
	}
	if blockId := block.GetHeader().GetBlockHash(); !bytes.Equal(qc.BlockId, blockId) {
		return fmt.Errorf("wrong qc block id [%v], expected [%v]", qc.BlockId, blockId)
	}

	// 2. get governance contract
	// because the validator set has changed after the generation switch,
	// so that validate by validators cannot be continue.
	governanceContract, err := governance.NewGovernanceContract(store, ledger).GetGovernanceContract()
	if err != nil {
		return err
	}
	var (
		quorumForQC uint64
		validators  []*consensus.GovernanceMember
	)
	epochSwitchHeight := governanceContract.GetNextSwitchHeight()
	if epochSwitchHeight+3 >= block.Header.BlockHeight {
		validators = governanceContract.GetLastValidators()
		quorumForQC = governanceContract.GetLastMinQuorumForQc()
	} else {
		validators = governanceContract.GetValidators()
		quorumForQC = governanceContract.GetMinQuorumForQc()
	}
	//2. get validators from governance contract
	//curValidators := governanceContract.GetValidators()
	newViewNum, votedBlockNum, err := countNumFromVotes(qc, validators, ac)
	if err != nil {
		return err
	}
	//minQuorumForQc := governanceContract.GetMinQuorumForQc()
	if qc.Level > 0 && qc.NewView && newViewNum < quorumForQC {
		return fmt.Errorf(fmt.Sprintf("vote new view num [%v] less than expected [%v]",
			newViewNum, quorumForQC))
	}
	if qc.Level > 0 && !qc.NewView && votedBlockNum < quorumForQC {
		return fmt.Errorf(fmt.Sprintf("vote block num [%v] less than expected [%v]",
			votedBlockNum, quorumForQC))
	}
	return nil
}

func validateVoteData(voteData *chainedbftpb.VoteData, validators []*consensus.GovernanceMember, ac protocol.AccessControlProvider) error {
	author := voteData.GetAuthor()
	authorIdx := voteData.GetAuthorIdx()
	if author == nil {
		return fmt.Errorf("author is nil")
	}

	// get validator by authorIdx
	var validator *consensus.GovernanceMember
	for _, v := range validators {
		if v.Index == int64(authorIdx) {
			validator = v
			break
		}
	}
	if validator == nil {
		return fmt.Errorf("msg index not in validators")
	}
	if validator.NodeId != string(author) {
		return fmt.Errorf("msg author not equal validator nodeid")
	}

	// check cert id
	if voteData.Signature == nil || voteData.Signature.Signer == nil {
		return fmt.Errorf("signer is nil")
	}

	//check sign
	sign := voteData.Signature
	voteData.Signature = nil
	defer func() {
		voteData.Signature = sign
	}()
	data, err := proto.Marshal(voteData)
	if err != nil {
		return fmt.Errorf("marshal payload failed, err %v", err)
	}
	err = utils.VerifyDataSign(data, sign, ac)
	if err != nil {
		return fmt.Errorf("verify signature failed, err %v", err)
	}
	return nil
}

func countNumFromVotes(qc *chainedbftpb.QuorumCert, currValidators []*consensus.GovernanceMember, ac protocol.AccessControlProvider) (uint64, uint64, error) {
	var newViewNum uint64 = 0
	var votedBlockNum uint64 = 0
	voteIdxes := make(map[uint64]bool, 0)
	//for each vote
	for _, vote := range qc.Votes {
		if vote == nil {
			return 0, 0, fmt.Errorf("nil Commits msg")
		}
		if err := validateVoteData(vote, currValidators, ac); err != nil {
			return 0, 0, fmt.Errorf("invalid commits, err %v", err)
		}
		// vote := msg.Payload.GetVoteMsg()
		if vote.Height != qc.Height || vote.Level != qc.Level {
			return 0, 0, fmt.Errorf("vote for wrong height:round:level [%v:%v], expected [%v:%v]",
				vote.Height, vote.Level, qc.Height, qc.Level)
		}
		if ok := voteIdxes[vote.AuthorIdx]; ok {
			return 0, 0, fmt.Errorf("duplicate vote index [%v] at height:round:level [%v:%v]",
				vote.AuthorIdx, vote.Height, vote.Level)
		}
		voteIdxes[vote.AuthorIdx] = true
		if vote.NewView && vote.BlockId == nil {
			newViewNum++
			continue
		}

		if qc.BlockId != nil && (bytes.Compare(vote.BlockId, qc.BlockId) < 0) {
			continue
		}
		votedBlockNum++
	}
	return newViewNum, votedBlockNum, nil
}
