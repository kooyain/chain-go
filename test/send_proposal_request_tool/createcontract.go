/*
Copyright (C) BABEC. All rights reserved.
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"chainmaker.org/chainmaker-go/utils"
	commonPb "chainmaker.org/chainmaker/pb-go/common"

	"github.com/spf13/cobra"
)

func CreateContractCMD() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "createContract",
		Short: "Create Contract",
		Long:  "Create Contract",
		RunE: func(_ *cobra.Command, _ []string) error {
			return createContract()
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&wasmPath, "wasm-path", "w", "../wasm/counter-go.wasm", "specify wasm path")
	flags.Int32VarP(&runTime, "run-time", "r", int32(commonPb.RuntimeType_GASM), "specify run time")
	flags.StringVarP(&abiPath, "abi-path", "", "", "specify wasm path")
	flags.StringVarP(&pairsString, "pairs", "", "", "specify pairs")
	flags.StringVarP(&pairsFile, "pairs-file", "", "", "specify pairs file, if used, set --pairs=\"\"")
	return cmd
}

func createContract() error {
	txId := utils.GetRandTxId()

	// 构造Payload
	if pairsString == "" {
		bytes, err := ioutil.ReadFile(pairsFile)
		if err != nil {
			panic(err)
		}
		pairsString = string(bytes)
	}
	var pairs []*commonPb.KeyValuePair
	err := json.Unmarshal([]byte(pairsString), &pairs)
	if err != nil {
		return err
	}

	//wasm
	wasmBin, err := ioutil.ReadFile(wasmPath)
	if err != nil {
		return err
	}

	method, pairs, err = makeCreateContractPairs("", abiPath, pairs, commonPb.RuntimeType(runTime))
	if err != nil {
		return fmt.Errorf("make pairs filure!")
	}
	if commonPb.RuntimeType(runTime) == commonPb.RuntimeType_EVM {
		wasmBin, err = hex.DecodeString(string(wasmBin))
	}

	//if commonPb.RuntimeType(runTime) == commonPb.RuntimeType_EVM {
	//	//fmt.Println("input : ", initParams)
	//	data := ""
	//	//对于参数的处理
	//	if initParams != "" {
	//		abiJsonData, err := ioutil.ReadFile(abiPath)
	//		//fmt.Println("abiPath : ", abiPath, " ---> abiJsonData: ", abiJsonData)
	//		if err != nil {
	//			return err
	//		}
	//		myAbi, _ := abi.JSON(strings.NewReader(string(abiJsonData)))
	//		addr := evm.BigToAddress(evm.FromDecimalString(initParams))
	//		dataByte, err := myAbi.Pack("", addr)
	//		if err != nil {
	//			return err
	//		}
	//		data = hex.EncodeToString(dataByte)
	//	}
	//	pairs = []*commonPb.KeyValuePair{
	//		{
	//			Key:   "data",
	//			Value: []byte(data),
	//		},
	//	}
	//	wasmBin, err = hex.DecodeString(string(wasmBin))
	//}
	//var pairs []*commonPb.KeyValuePair
	payload, _ := utils.GenerateInstallContractPayload(contractName, "1.0.0", commonPb.RuntimeType(runTime), wasmBin, pairs)

	//if endorsement, err := acSign(payload); err == nil {
	//	payload.Endorsement = endorsement
	//} else {
	//	return err
	//}

	resp, err = proposalRequest(sk3, client, commonPb.TxType_INVOKE_CONTRACT,
		chainId, txId, payload)
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
