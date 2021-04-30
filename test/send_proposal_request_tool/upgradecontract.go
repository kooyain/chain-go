/*
Copyright (C) BABEC. All rights reserved.
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	commonPb "chainmaker.org/chainmaker-go/pb/protogo/common"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"chainmaker.org/chainmaker-go/utils"
	"github.com/gogo/protobuf/proto"
	"github.com/spf13/cobra"
)

func UpgradeContractCMD() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgradeContract",
		Short: "Upgrade Contract",
		Long:  "Upgrade Contract",
		RunE: func(_ *cobra.Command, _ []string) error {
			return upgradeContract()
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&wasmPath, "wasm-path", "w", "../wasm/counter-go.wasm", "specify wasm path")
	flags.Int32VarP(&runTime, "run-time", "r", int32(commonPb.RuntimeType_GASM), "specify run time")
	flags.StringVarP(&version, "version", "v", "2.0.0", "specify contract version")

	return cmd
}

func upgradeContract() error {
	txId := utils.GetRandTxId()

	wasmBin, err := ioutil.ReadFile(wasmPath)
	if err != nil {
		return err
	}

	var pairs []*commonPb.KeyValuePair

	method := commonPb.ManageUserContractFunction_UPGRADE_CONTRACT.String()

	payload := &commonPb.ContractMgmtPayload{
		ChainId: chainId,
		ContractId: &commonPb.ContractId{
			ContractName:    contractName,
			ContractVersion: version,
			RuntimeType:     commonPb.RuntimeType(runTime),
		},
		Method:      method,
		Parameters:  pairs,
		ByteCode:    wasmBin,
		Endorsement: nil,
	}

	if endorsement, err := acSign(payload); err == nil {
		payload.Endorsement = endorsement
	} else {
		return err
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err = proposalRequest(sk3, client, commonPb.TxType_MANAGE_USER_CONTRACT,
		chainId, txId, payloadBytes)
	if err != nil {
		return err
	}

	result := &Result{
		Code:    resp.Code,
		Message: resp.Message,
		TxId:    txId,
	}
	bytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	fmt.Println(string(bytes))

	return nil
}
