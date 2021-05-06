/*
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chainedbft

import (
	"fmt"

	"chainmaker.org/chainmaker-go/pb/protogo/net"

	"chainmaker.org/chainmaker-go/common/msgbus"
	"chainmaker.org/chainmaker-go/consensus/chainedbft/utils"
	chainedbftpb "chainmaker.org/chainmaker-go/pb/protogo/consensus/chainedbft"
	"github.com/gogo/protobuf/proto"
)

//signAndMarshal signs the consensus payload and marshal consensus message including signature
func (cbi *ConsensusChainedBftImpl) signAndMarshal(payload *chainedbftpb.ConsensusPayload, internal bool) ([]byte, error) {
	consensusMessage := &chainedbftpb.ConsensusMsg{
		Payload:   payload,
		SignEntry: nil,
	}
	if err := utils.SignConsensusMsg(consensusMessage, cbi.chainConf.ChainConfig().Crypto.Hash, cbi.singer); err != nil {
		return nil, fmt.Errorf("sign consensus message failed, err %v", err)
	}
	cbi.logger.Debugf("signAndMarshal, consensus msg %v", payload.String())
	if internal {
		//send it to self, no need to marshal
		cbi.internalMsgCh <- consensusMessage
	}
	consensusData, err := proto.Marshal(consensusMessage)
	if err != nil {
		return nil, fmt.Errorf("marshal consensus message failed, err %v", err)
	}
	return consensusData, nil
}

//signAndBroadcast signs the consensus message and broadcasts it to consensus group
func (cbi *ConsensusChainedBftImpl) signAndBroadcast(payload *chainedbftpb.ConsensusPayload) {
	consensusData, err := cbi.signAndMarshal(payload, true)
	if err != nil {
		cbi.logger.Errorf("sign payload failed, err %v", err)
		return
	}
	peers := cbi.smr.peers()
	cbi.logger.Debugf("peers count: %d in signAndBroadcast", len(peers))
	for _, peer := range peers {
		if peer.index == cbi.selfIndexInEpoch {
			continue
		}
		msg := &net.NetMsg{
			Payload: consensusData,
			Type:    net.NetMsg_CONSENSUS_MSG,
			To:      peer.id,
		}
		cbi.logger.Debugf("broadcast proposal msg ...")
		go cbi.msgbus.Publish(msgbus.SendConsensusMsg, msg)
	}
}

//signAndSendToPeer signs the consensus message and unicast it to the specified peer
func (cbi *ConsensusChainedBftImpl) signAndSendToPeer(payload *chainedbftpb.ConsensusPayload, index uint64) {
	consensusData, err := cbi.signAndMarshal(payload, false)
	if err != nil {
		cbi.logger.Errorf("sign payload failed, err %v", err)
		return
	}
	cbi.sendToPeer(consensusData, index)
}

//sendToPeer sends consensus data to specified peer
func (cbi *ConsensusChainedBftImpl) sendToPeer(consensusData []byte, index uint64) {
	peer := cbi.smr.getPeerByIndex(index)
	if peer == nil {
		cbi.logger.Errorf("get peer with index %v failed", cbi.selfIndexInEpoch, index)
		return
	}
	msg := &net.NetMsg{
		To:      peer.id,
		Type:    net.NetMsg_CONSENSUS_MSG,
		Payload: consensusData,
	}
	go cbi.msgbus.Publish(msgbus.SendConsensusMsg, msg)
}
