/*
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chainedbft

import (
	"chainmaker.org/chainmaker-go/consensus/chainedbft/liveness"
	safetyrules "chainmaker.org/chainmaker-go/consensus/chainedbft/safety_rules"
	"chainmaker.org/chainmaker-go/consensus/chainedbft/utils"
	"chainmaker.org/chainmaker-go/logger"
	"chainmaker.org/chainmaker/pb-go/common"
	chainedbftpb "chainmaker.org/chainmaker/pb-go/consensus/chainedbft"
)

//chainedbftSMR manages current smr of consensus at height and level
type chainedbftSMR struct {
	info        *contractInfo              // The governance contract info that be cached
	state       chainedbftpb.ConsStateType // The current consensus state of the local node
	paceMaker   *liveness.Pacemaker        // govern the advancement of levels and height of the local node
	chainStore  *chainStore                // access data on the chain and in the cache, commit block data on the chain
	safetyRules *safetyrules.SafetyRules   // validate incoming qc and block and update its' state to the newest

	logger *logger.CMLogger
	server *ConsensusChainedBftImpl
}

//newChainedBftSMR returns an instance of consensus smr
func newChainedBftSMR(server *ConsensusChainedBftImpl, epoch *epochManager) (*chainedbftSMR, error) {
	var store = server.chainStore
	smr := &chainedbftSMR{
		server:      server,
		logger:      server.logger,
		chainStore:  server.chainStore,
		paceMaker:   liveness.NewPacemaker(server.logger, epoch.index, epoch.createHeight, epoch.epochId, server.timerService),
		safetyRules: safetyrules.NewSafetyRules(server.logger, store.blockPool, store.blockChainStore),
	}

	smr.info = newContractInfo(epoch.governanceContract)
	smr.logger.Debugf("currPeers [%v], lastPeers [%v], switchHeight: %d",
		epoch.governanceContract.Validators, epoch.governanceContract.LastValidators, epoch.governanceContract.NextSwitchHeight)
	return smr, nil
}

//forwardNewHeightIfNeed resets the consensus smr by chainStore, and update state to ConsStateType_NEW_HEIGHT
func (cs *chainedbftSMR) forwardNewHeightIfNeed() error {
	lastBlock := cs.chainStore.getCurrentCertifiedBlock()
	cs.logger.Debugf("forwardNewHeightIfNeed to chainStore state, smr height [%v],"+
		" qcBlock height [%v]", cs.getHeight(), lastBlock.Header.BlockHeight)
	if cs.getHeight() > 0 && cs.getHeight() != lastBlock.Header.BlockHeight {
		cs.logger.Warnf("mismatched height [%v], expected [%v]",
			lastBlock.Header.BlockHeight, cs.getHeight())
		return nil
	}

	cs.state = chainedbftpb.ConsStateType_NEW_HEIGHT
	level, err := utils.GetLevelFromBlock(lastBlock)
	if err != nil {
		cs.logger.Errorf("get level from block error: %s, block %v", err, lastBlock)
		return err
	}
	cs.safetyRules.SetLastCommittedLevel(level)
	return nil
}

func (cs *chainedbftSMR) updateContractInfo(epoch *epochManager) {
	cs.info = newContractInfo(epoch.governanceContract)
}

func (cs *chainedbftSMR) updateState(newState chainedbftpb.ConsStateType) {
	//cs.Lock()
	//defer cs.Unlock()
	cs.state = newState
}

func (cs *chainedbftSMR) peers(blkHeight uint64) []*peer {
	return cs.info.getPeers(blkHeight)
}

func (cs *chainedbftSMR) getPeerByIndex(index uint64, blkHeight uint64) *peer {
	return cs.info.getPeerByIndex(index, blkHeight)
}

func (cs *chainedbftSMR) isValidIdx(index uint64, blkHeight uint64) bool {
	return cs.info.isValidIdx(index, blkHeight)
}

func (cs *chainedbftSMR) min(qcHeight uint64) int {
	return cs.info.minQuorumForQc(qcHeight)
}

func (cs *chainedbftSMR) getPeers(blkHeight uint64) []*peer {
	return cs.info.getPeers(blkHeight)
}

func (cs *chainedbftSMR) getLastVote() (uint64, *chainedbftpb.ConsensusPayload) {
	return cs.safetyRules.GetLastVoteLevel(), cs.safetyRules.GetLastVoteMsg()
}

func (cs *chainedbftSMR) setLastVote(vote *chainedbftpb.ConsensusPayload, level uint64) {
	cs.safetyRules.SetLastVote(vote, level)
}

func (cs *chainedbftSMR) safeNode(proposal *chainedbftpb.ProposalData) error {
	return cs.safetyRules.SafeNode(proposal)
}

func (cs *chainedbftSMR) updateLockedQC(qc *chainedbftpb.QuorumCert) {
	cs.safetyRules.UpdateLockedQC(qc)
}

func (cs *chainedbftSMR) commitRules(qc *chainedbftpb.QuorumCert) (commit bool, commitBlock *common.Block, commitLevel uint64) {
	return cs.safetyRules.CommitRules(qc)
}

func (cs *chainedbftSMR) setLastCommittedLevel(level uint64) {
	cs.safetyRules.SetLastCommittedLevel(level)
}

func (cs *chainedbftSMR) getLastCommittedLevel() uint64 {
	return cs.safetyRules.GetLastCommittedLevel()
}

func (cs *chainedbftSMR) getHeight() uint64 {
	return cs.paceMaker.GetHeight()
}
func (cs *chainedbftSMR) getEpochId() uint64 {
	return cs.paceMaker.GetEpochId()
}

func (cs *chainedbftSMR) getCurrentLevel() uint64 {
	return cs.paceMaker.GetCurrentLevel()
}

func (cs *chainedbftSMR) processLocalTimeout(level uint64) bool {
	return cs.paceMaker.ProcessLocalTimeout(level)
}

func (cs *chainedbftSMR) getHighestTCLevel() uint64 {
	return cs.paceMaker.GetHighestTCLevel()
}

// processCertificates Update the status of local Pacemaker with the latest received QC;
// height The height of the received block
// hqcLevel The highest QC level in local node;
// htcLevel The received tcLevel
// hcLevel The latest committed QC level in local node.
// Returns true if the consensus goes to the next level, otherwise false.
//func (cs *chainedbftSMR) processCertificates(height, hqcLevel, htcLevel, hcLevel uint64) bool {
func (cs *chainedbftSMR) processCertificates(qc *chainedbftpb.QuorumCert, tc *chainedbftpb.QuorumCert, hcLevel uint64) bool {
	return cs.paceMaker.ProcessCertificates(qc, tc, hcLevel)
}

func (cs *chainedbftSMR) updateTC(tc *chainedbftpb.QuorumCert) {
	cs.paceMaker.UpdateTC(tc)
}

func (cs *chainedbftSMR) getTC() *chainedbftpb.QuorumCert {
	return cs.paceMaker.GetTC()
}
