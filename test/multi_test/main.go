/*
Copyright (C) BABEC. All rights reserved.
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"chainmaker.org/chainmaker-go/accesscontrol"
	"chainmaker.org/chainmaker-go/test/common"
	"chainmaker.org/chainmaker-go/utils"
	"chainmaker.org/chainmaker/common/ca"
	"chainmaker.org/chainmaker/common/crypto"
	"chainmaker.org/chainmaker/common/crypto/asym"
	acPb "chainmaker.org/chainmaker/pb-go/accesscontrol"
	apiPb "chainmaker.org/chainmaker/pb-go/api"
	commonPb "chainmaker.org/chainmaker/pb-go/common"
	discoveryPb "chainmaker.org/chainmaker/pb-go/discovery"
	"chainmaker.org/chainmaker/pb-go/syscontract"
	"chainmaker.org/chainmaker/protocol"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"google.golang.org/grpc"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	logTempMarshalPayLoadFailed     = "marshal payload failed, %s"
	logTempUnmarshalBlockInfoFailed = "blockInfo unmarshal error %s\n"
	logTempSendTx                   = "send tx resp: code:%d, msg:%s, payload:%+v\n"
	logTempSendBlock                = "send tx resp: code:%d, msg:%s, blockInfo:%+v\n"
	fieldWithRWSet                  = "withRWSet"
)

const (
	CHAIN1         = "chain1"
	IP             = "localhost"
	Port           = 12301
	certPathPrefix = "../../config"
	userKeyPath    = certPathPrefix + "/crypto-config/wx-org1.chainmaker.org/user/client1/client1.tls.key"
	userCrtPath    = certPathPrefix + "/crypto-config/wx-org1.chainmaker.org/user/client1/client1.tls.crt"
	//userCrtPath2    = certPathPrefix + "/crypto-config/wx-org2.chainmaker.org/user/client1/client1.tls.crt"
	//userCrtPath3    = certPathPrefix + "/crypto-config/wx-org3.chainmaker.org/user/client1/client1.tls.crt"
	orgId       = "wx-org1.chainmaker.org"
	prePathFmt  = certPathPrefix + "/crypto-config/wx-org%s.chainmaker.org/user/admin1/"
	prePathFmt2 = certPathPrefix + "/crypto-config/wx-org%s.chainmaker.org/user/admin2/"
	prePathFmt3 = certPathPrefix + "/crypto-config/wx-org%s.chainmaker.org/user/admin3/"
)

var (
	WasmPath        = ""
	WasmUpgradePath = ""
	contractName    = ""
	runtimeType     = commonPb.RuntimeType_WASMER
	multiOrgId      = "wx-org1.chainmaker.org"
	multiOrg3Id     = "wx-org3.chainmaker.org"
	txId            = ""
	timestamp       int64
	timestampBak    int64
)

var caPaths = []string{certPathPrefix + "/crypto-config/wx-org1.chainmaker.org/ca"}

// vm wasmer 整体功能测试，合约创建、升级、执行、查询、冻结、解冻、吊销、交易区块的查询、链配置信息的查询
func main() {
	common.SetCertPathPrefix(certPathPrefix)

	//initGasmTest()
	initWasmerTest()
	runTest()

}

func runTest() {
	var (
		conn   *grpc.ClientConn
		client apiPb.RpcNodeClient
		sk3    crypto.PrivateKey
		err    error
	)
	// init
	{
		conn, err = initGRPCConnect(true)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer conn.Close()

		client = apiPb.NewRpcNodeClient(conn)

		file, err := ioutil.ReadFile(userKeyPath)
		if err != nil {
			panic(err)
		}

		sk3, err = asym.PrivateKeyFromPEM(file, nil)
		if err != nil {
			panic(err)
		}
	}
	txId = utils.GetRandTxId()
	//timestamp = 1628580218
	timestamp = time.Now().Unix()
	fmt.Println("timestamp", timestamp)
	testMultiSign(sk3, &client, "", CHAIN1)

	time.Sleep(time.Second * 3)
	multiOrgId = "wx-org2.chainmaker.org"
	testMultiSignVote(sk3, &client, "", CHAIN1)
	multiOrgId = "wx-org3.chainmaker.org"
	testMultiSignVote(sk3, &client, "", CHAIN1)
	multiOrgId = "wx-org4.chainmaker.org"
	testMultiSignVote(sk3, &client, "", CHAIN1)

	// 1) 合约创建
	//testCreate(sk3, &client, CHAIN1)
	//time.Sleep(4 * time.Second)
	//
	// 2) 执行合约
	time.Sleep(time.Second * 4)
	testUpgradeInvokeSum(sk3, &client, CHAIN1) // method [sum] not export, 合约升级后则有
	//
	//txId = testInvokeFactSave(sk3, &client, CHAIN1)
	//time.Sleep(2 * time.Second)
	//testWaitTx(sk3, &client, CHAIN1, txId)
	//
	//// 3) 合约查询
	//_, result := testQueryFindByHash(sk3, &client, CHAIN1)
	//if string(result) != "{\"file_hash\":\"b4018d181b6f\",\"file_name\":\"长安链chainmaker\",\"time\":\"1615188470000\"}" {
	//	fmt.Println("query result:", string(result))
	//	log.Panicf("query error")
	//} else {
	//	fmt.Println("    【testQueryFindByHash】 pass")
	//}

	// 4) 根据TxId查交易
	//testGetTxByTxId(sk3, &client, txId, CHAIN1)

	//多签交易

	// 5) 根据区块高度查区块，若height为max，表示查当前区块
	//hash := testGetBlockByHeight(sk3, &client, CHAIN1, math.MaxUint64)
	//
	//// 6) 根据区块高度查区块（包含读写集），若height为-1，表示查当前区块
	//testGetBlockWithTxRWSetsByHeight(sk3, &client, CHAIN1, math.MaxUint64)
	//
	//// 7) 根据区块哈希查区块
	//testGetBlockByHash(sk3, &client, CHAIN1, hash)
	//
	//// 8) 根据区块哈希查区块（包含读写集）
	//testGetBlockWithTxRWSetsByHash(sk3, &client, CHAIN1, hash)
	//
	//// 9) 根据TxId查区块
	//testGetBlockByTxId(sk3, &client, txId, CHAIN1)
	//
	//// 10) 查询最新配置块
	//testGetLastConfigBlock(sk3, &client, CHAIN1)
	//
	//// 11) 查询最新区块
	//testGetLastBlock(sk3, &client, CHAIN1)
	//
	//// 12) 查询链信息
	//testGetChainInfo(sk3, &client, CHAIN1)
	//
	//// 13) 合约升级
	//testUpgrade(sk3, &client, CHAIN1)
	//time.Sleep(4 * time.Second)
	//
	//// 14) 合约执行
	//testUpgradeInvokeSum(sk3, &client, CHAIN1)
	//
	//// 15) 批量执行
	//txId = testInvokeFactSave(sk3, &client, CHAIN1)
	//time.Sleep(2 * time.Second)
	//testWaitTx(sk3, &client, CHAIN1, txId)
	//testPerformanceModeTransfer(sk3, &client, CHAIN1)
	//time.Sleep(5 * time.Second)
	//
	//// 16) 功能测试
	//testInvokeFunctionalVerify(sk3, &client, CHAIN1)
	//time.Sleep(5 * time.Second)
	//
	//// 17) KV迭代器测试
	//testKvIterator(sk3, &client)
	//
	//// 18) 冻结、解冻、吊销用户合约功能测试
	//testFreezeOrUnfreezeOrRevokeFlow(sk3, client)

	fmt.Println("    【runTest】 pass", "txId", txId)
}
func initWasmerTest() {
	WasmPath = "../wasm/rust-func-verify-2.0.0.wasm"
	WasmUpgradePath = WasmPath
	contractName = "contract106"
	runtimeType = commonPb.RuntimeType_WASMER
	printConfig("wasmer")
}
func initGasmTest() {
	WasmPath = "../wasm/go-fact-2.0.0.wasm"
	WasmUpgradePath = WasmPath
	contractName = "contract201"
	runtimeType = commonPb.RuntimeType_GASM
	printConfig("gasm")
}

func printConfig(wasmType string) {
	fmt.Printf("=========init %s=========\n", wasmType)
	fmt.Println("  wasm path         : ", WasmPath)
	fmt.Println("  wasm upgrade path : ", WasmUpgradePath)
	fmt.Println("  wasm contract name: ", contractName)
	fmt.Println("  wasm runtime type : ", runtimeType)
	fmt.Println()
}

func testKvIterator(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient) {
	testInvokeMethod(sk3, client, "test_put_state")
	time.Sleep(time.Second * 4)
	r := testQueryMethod(sk3, client, "test_kv_iterator")
	time.Sleep(time.Second * 4)
	if "15" != string(r) {
		panic("testKvIterator error count!=15 count=" + string(r))
	} else {
		fmt.Println("    【testKvIterator】 pass")
	}
}
func testPerformanceModeTransfer(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) {
	fmt.Println("==============================================")
	fmt.Println("==============================================")
	fmt.Println("==============start batch invoke==============")
	fmt.Println("==============================================")
	fmt.Println("==============================================")
	start := utils.CurrentTimeMillisSeconds()
	wg := sync.WaitGroup{}
	for j := 0; j < 10; j++ {
		wg.Add(1)
		go func() {
			for j := 0; j < 10; j++ {
				testInvokeFactSave(sk3, client, CHAIN1)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	end := utils.CurrentTimeMillisSeconds()
	spend := end - start
	fmt.Println("发送100个交易所花时间", spend, "ms")
}
func testFreezeOrUnfreezeOrRevokeFlow(sk3 crypto.PrivateKey, client apiPb.RpcNodeClient) {
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Println("==============test freeze unfreeze revoke flow==============")
	fmt.Println("============================================================")
	fmt.Println("============================================================")

	//执行合约
	testInvokeFactSave(sk3, &client, CHAIN1)
	testQueryFindByHash(sk3, &client, CHAIN1)
	time.Sleep(4 * time.Second)

	// 冻结
	common.FreezeContract(sk3, &client, CHAIN1, contractName, runtimeType)
	//testFreezeOrUnfreezeOrRevoke(sk3, &client, CHAIN1, syscontract.ContractManageFunction_FREEZE_CONTRACT.String())
	time.Sleep(4 * time.Second)
	testInvokeFactSave(sk3, &client, CHAIN1)
	testQueryFindByHash(sk3, &client, CHAIN1)
	time.Sleep(4 * time.Second)

	// 解冻
	common.UnfreezeContract(sk3, &client, CHAIN1, contractName, runtimeType)
	//testFreezeOrUnfreezeOrRevoke(sk3, &client, CHAIN1, syscontract.ContractManageFunction_UNFREEZE_CONTRACT.String())
	time.Sleep(4 * time.Second)
	testInvokeFactSave(sk3, &client, CHAIN1)
	testQueryFindByHash(sk3, &client, CHAIN1)
	time.Sleep(4 * time.Second)

	// 冻结
	common.FreezeContract(sk3, &client, CHAIN1, contractName, runtimeType)
	//testFreezeOrUnfreezeOrRevoke(sk3, &client, CHAIN1, syscontract.ContractManageFunction_FREEZE_CONTRACT.String())
	time.Sleep(4 * time.Second)
	testInvokeFactSave(sk3, &client, CHAIN1)
	testQueryFindByHash(sk3, &client, CHAIN1)
	time.Sleep(4 * time.Second)

	// 解冻
	common.UnfreezeContract(sk3, &client, CHAIN1, contractName, runtimeType)
	//testFreezeOrUnfreezeOrRevoke(sk3, &client, CHAIN1, syscontract.ContractManageFunction_UNFREEZE_CONTRACT.String())
	time.Sleep(4 * time.Second)
	testInvokeFactSave(sk3, &client, CHAIN1)
	testQueryFindByHash(sk3, &client, CHAIN1)
	time.Sleep(4 * time.Second)

	// 冻结
	common.FreezeContract(sk3, &client, CHAIN1, contractName, runtimeType)
	//testFreezeOrUnfreezeOrRevoke(sk3, &client, CHAIN1, syscontract.ContractManageFunction_FREEZE_CONTRACT.String())
	time.Sleep(4 * time.Second)
	// 吊销
	common.RevokeContract(sk3, &client, CHAIN1, contractName, runtimeType)
	//testFreezeOrUnfreezeOrRevoke(sk3, &client, CHAIN1, syscontract.ContractManageFunction_UNFREEZE_CONTRACT.String())
	time.Sleep(4 * time.Second)
	testInvokeFactSave(sk3, &client, CHAIN1)
	testQueryFindByHash(sk3, &client, CHAIN1)
	time.Sleep(4 * time.Second)
	common.FreezeContract(sk3, &client, CHAIN1, contractName, runtimeType)
	//testFreezeOrUnfreezeOrRevoke(sk3, &client, CHAIN1, syscontract.ContractManageFunction_FREEZE_CONTRACT.String())
	time.Sleep(4 * time.Second)
	common.UnfreezeContract(sk3, &client, CHAIN1, contractName, runtimeType)
	//testFreezeOrUnfreezeOrRevoke(sk3, &client, CHAIN1, syscontract.ContractManageFunction_UNFREEZE_CONTRACT.String())
	time.Sleep(4 * time.Second)
}

func testMultiSign(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, contractName, chainId string) []byte {
	fmt.Println("========================================================================================================")
	fmt.Println("========================================================================================================")
	fmt.Println("============================================testMultiSign===============================================")
	fmt.Println("========================================================================================================")
	fmt.Println("========================================================================================================")

	payload := initPayload()

	resp := common.ProposalMultiRequest(sk3, client, payload.TxType,
		chainId, payload.TxId, payload, []int{1}, timestamp)

	fmt.Println("testMultiSign timestamp", timestamp)
	fmt.Println(resp)
	return nil
}

func initPayload() *commonPb.Payload {
	wasmBin, _ := ioutil.ReadFile(WasmPath)
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   syscontract.MultiReq_SYS_CONTRACT_NAME.String(),
			Value: []byte(syscontract.SystemContract_CONTRACT_MANAGE.String()),
		},
		{
			Key:   syscontract.MultiReq_SYS_METHOD.String(),
			Value: []byte(syscontract.ContractManageFunction_INIT_CONTRACT.String()),
		},
		{
			Key:   syscontract.InitContract_CONTRACT_NAME.String(),
			Value: []byte(contractName),
		},
		{
			Key:   syscontract.InitContract_CONTRACT_VERSION.String(),
			Value: []byte("1.0"),
		},
		{
			Key:   syscontract.InitContract_CONTRACT_BYTECODE.String(),
			Value: wasmBin,
		},
		{
			Key:   syscontract.InitContract_CONTRACT_RUNTIME_TYPE.String(),
			Value: []byte(runtimeType.String()),
		},
	}

	payload := &commonPb.Payload{
		TxType:       commonPb.TxType_INVOKE_CONTRACT,
		ContractName: syscontract.SystemContract_MULTI_SIGN.String(),
		Method:       syscontract.MultiSignFunction_REQ.String(),
		Parameters:   pairs,
		TxId:         txId,
		ChainId:      CHAIN1,
	}
	return payload
}

func initPayloadTimestamp() *commonPb.Payload {
	wasmBin, _ := ioutil.ReadFile(WasmPath)
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "sysContractName",
			Value: []byte(syscontract.SystemContract_CONTRACT_MANAGE.String()),
		},
		{
			Key:   "sysMethod",
			Value: []byte(syscontract.ContractManageFunction_INIT_CONTRACT.String()),
		},
		{
			Key:   syscontract.InitContract_CONTRACT_NAME.String(),
			Value: []byte(contractName),
		},
		{
			Key:   syscontract.InitContract_CONTRACT_VERSION.String(),
			Value: []byte("1.0"),
		},
		{
			Key: syscontract.InitContract_CONTRACT_BYTECODE.String(),
			//Value: nil,
			Value: wasmBin,
		},
		{
			Key:   syscontract.InitContract_CONTRACT_RUNTIME_TYPE.String(),
			Value: []byte(runtimeType.String()),
		},
	}

	payload := &commonPb.Payload{
		TxType:       commonPb.TxType_INVOKE_CONTRACT,
		ContractName: syscontract.SystemContract_MULTI_SIGN.String(),
		Method:       syscontract.MultiSignFunction_REQ.String(),
		Parameters:   pairs,
		Timestamp:    timestampBak,
		TxId:         txId,
		ChainId:      CHAIN1,
	}
	return payload
}
func GetSigner(sk3 crypto.PrivateKey, sender *acPb.Member) protocol.SigningMember {
	skPEM, err := sk3.String()
	if err != nil {
		log.Fatalf("get sk PEM failed, %s", err.Error())
	}
	//fmt.Printf("skPEM: %s\n", skPEM)

	m, err := accesscontrol.MockAccessControl().NewMemberFromCertPem(sender.OrgId, string(sender.MemberInfo))
	if err != nil {
		panic(err)
	}

	signer, err := accesscontrol.MockAccessControl().NewSigningMember(m, skPEM, "")
	if err != nil {
		panic(err)
	}
	return signer
}

func testMultiSignVote(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, contractName, chainId string) []byte {
	fmt.Println("========================================================================================================")
	fmt.Println("========================================================================================================")
	fmt.Println("==========================================testMultiSignVote ============================================")
	fmt.Println("========================================================================================================")
	fmt.Println("========================================================================================================")

	payload1 := initPayload()
	payload1.Timestamp = timestamp
	payloadBytes, err := payload1.Marshal()
	fmt.Printf("testMultiSignVote1 payload md5 is %x \n", md5.Sum(payloadBytes))
	if err != nil {
		panic(err)
	}
	var (
		certPathPrefix = "../../config"
		admin1KeyPath  = certPathPrefix + "/crypto-config/" + multiOrgId + "/user/admin1/admin1.tls.key"
		admin1CrtPath  = certPathPrefix + "/crypto-config/" + multiOrgId + "/user/admin1/admin1.tls.crt"
	)

	var msviByte []byte
	{
		admin1File, err := ioutil.ReadFile(admin1CrtPath)
		if err != nil {
			panic(err)
		}
		fadminKeyFile, err := ioutil.ReadFile(admin1KeyPath)
		if err != nil {
			panic(err)
		}
		admin1 := &acPb.Member{
			OrgId:      multiOrgId,
			MemberInfo: admin1File,
		}
		skAdmin1, err := asym.PrivateKeyFromPEM(fadminKeyFile, nil)
		signerAdmin1 := GetSigner(skAdmin1, admin1)
		signerAdmin1Bytes, err := signerAdmin1.Sign("SHA256", payloadBytes) //modify
		//signerAdmin1Bytes, err := signerAdmin1.Sign("SM3", payloadBytes) //modify
		if err != nil {
			log.Fatalf("sign failed, %s", err.Error())
			os.Exit(0)
		}

		ee := &commonPb.EndorsementEntry{
			Signer:    admin1,
			Signature: signerAdmin1Bytes,
		}

		msvi := &syscontract.MultiSignVoteInfo{
			Vote:        syscontract.VoteStatus_AGREE,
			Endorsement: ee,
		}
		msviByte, _ = msvi.Marshal()

	}
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   syscontract.MultiVote_VOTE_INFO.String(),
			Value: msviByte,
		},
		{
			Key:   syscontract.MultiVote_TX_ID.String(),
			Value: []byte(payload1.TxId),
		},
	}

	payload := &commonPb.Payload{
		TxType:       commonPb.TxType_INVOKE_CONTRACT,
		ContractName: syscontract.SystemContract_MULTI_SIGN.String(),
		Method:       syscontract.MultiSignFunction_VOTE.String(),
		Parameters:   pairs,
	}

	//resp := common.ProposalRequest(sk3, client, payload.TxType,
	//chainId, "", payload, nil)
	resp := common.ProposalMultiRequest(sk3, client, payload.TxType,
		chainId, "", payload, nil, time.Now().Unix())

	fmt.Println(resp)
	return nil
}

func testMultiSignQuery(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, contractName, chainId string) []byte {
	fmt.Println("========================================================================================================")
	fmt.Println("========================================================================================================")
	fmt.Println("==========================================testMultiSignQuery ===========================================")
	fmt.Println("========================================================================================================")
	fmt.Println("========================================================================================================")

	payload1 := initPayloadTimestamp()
	payloadBytes, err := payload1.Marshal()
	if err != nil {
		panic(err)
	}
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "multiPayload",
			Value: payloadBytes,
		},
	}

	payload := &commonPb.Payload{
		TxType:       commonPb.TxType_INVOKE_CONTRACT,
		ContractName: syscontract.SystemContract_MULTI_SIGN.String(),
		Method:       syscontract.MultiSignFunction_QUERY.String(),
		Parameters:   pairs,
	}

	resp := common.ProposalRequest(sk3, client, payload.TxType,
		chainId, "", payload, nil)

	fmt.Println(resp)
	return nil
}

func testGetBlockByTxId(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, txId, chainId string) {
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Println("========get block by txId ", txId, "===============")
	fmt.Println("============================================================")
	fmt.Println("============================================================")

	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "txId",
			Value: []byte(txId),
		},
		{
			Key:   fieldWithRWSet,
			Value: []byte("false"),
		},
	}

	payloadBytes := common.ConstructQueryPayload(syscontract.SystemContract_CHAIN_QUERY.String(), "GET_BLOCK_BY_TX_ID", pairs)

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, txId, payloadBytes, nil)

	blockInfo := &commonPb.BlockInfo{}
	err := proto.Unmarshal(resp.ContractResult.Result, blockInfo)
	if err != nil {
		fmt.Printf(logTempUnmarshalBlockInfoFailed, err)
		os.Exit(0)
	}
	fmt.Printf(logTempSendBlock, resp.ContractResult.Code, resp.ContractResult.Message, blockInfo)
}

func testGetBlockByHeight(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string, height uint64) string {
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Println("========get block by height ", height, "===============")
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Printf("\n============ get block by height [%d] ============\n", height)
	// 构造Payload

	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "blockHeight",
			Value: []byte(strconv.FormatUint(height, 10)),
		},
		{
			Key:   fieldWithRWSet,
			Value: []byte("false"),
		},
	}

	payloadBytes := common.ConstructQueryPayload(syscontract.SystemContract_CHAIN_QUERY.String(), "GET_BLOCK_BY_HEIGHT", pairs)

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, "", payloadBytes, nil)

	blockInfo := &commonPb.BlockInfo{}
	err := proto.Unmarshal(resp.ContractResult.Result, blockInfo)
	if err != nil {
		fmt.Printf(logTempUnmarshalBlockInfoFailed, err)
		os.Exit(0)
	}
	fmt.Printf(logTempSendBlock, resp.ContractResult.Code, resp.ContractResult.Message, blockInfo)

	return hex.EncodeToString(blockInfo.Block.Header.BlockHash)
}

func testGetBlockWithTxRWSetsByHeight(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string, height uint64) string {
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Println("========get block with txRWsets by height ", height, "===============")
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Printf("\n============ get block with txRWsets by height [%d] ============\n", height)
	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "blockHeight",
			Value: []byte(strconv.FormatUint(height, 10)),
		},
	}

	payloadBytes := common.ConstructQueryPayload(syscontract.SystemContract_CHAIN_QUERY.String(), "GET_BLOCK_WITH_TXRWSETS_BY_HEIGHT", pairs)

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, "", payloadBytes, nil)

	blockInfo := &commonPb.BlockInfo{}
	err := proto.Unmarshal(resp.ContractResult.Result, blockInfo)
	if err != nil {
		fmt.Printf(logTempUnmarshalBlockInfoFailed, err)
		os.Exit(0)
	}
	fmt.Printf(logTempSendBlock, resp.ContractResult.Code, resp.ContractResult.Message, blockInfo)

	return hex.EncodeToString(blockInfo.Block.Header.BlockHash)
}

func testGetBlockByHash(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string, hash string) {
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Println("========get block by hash ", hash, "===============")
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Printf("\n============ get block by hash [%s] ============\n", hash)
	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "blockHash",
			Value: []byte(hash),
		},
		{
			Key:   fieldWithRWSet,
			Value: []byte("false"),
		},
	}

	payloadBytes := common.ConstructQueryPayload(syscontract.SystemContract_CHAIN_QUERY.String(), "GET_BLOCK_BY_HASH", pairs)

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, "", payloadBytes, nil)

	blockInfo := &commonPb.BlockInfo{}
	err := proto.Unmarshal(resp.ContractResult.Result, blockInfo)
	if err != nil {
		fmt.Printf(logTempUnmarshalBlockInfoFailed, err)
		os.Exit(0)
	}
	fmt.Printf(logTempSendBlock, resp.ContractResult.Code, resp.ContractResult.Message, blockInfo)
}

func testGetBlockWithTxRWSetsByHash(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string, hash string) {
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Println("========get block with txRWsets by hash ", hash, "===============")
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Printf("\n============ get block with txRWsets by hash [%s] ============\n", hash)
	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "blockHash",
			Value: []byte(hash),
		},
	}

	payloadBytes := common.ConstructQueryPayload(syscontract.SystemContract_CHAIN_QUERY.String(), "GET_BLOCK_WITH_TXRWSETS_BY_HASH", pairs)

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, "", payloadBytes, nil)

	blockInfo := &commonPb.BlockInfo{}
	err := proto.Unmarshal(resp.ContractResult.Result, blockInfo)
	if err != nil {
		fmt.Printf(logTempUnmarshalBlockInfoFailed, err)
		os.Exit(0)
	}
	fmt.Printf(logTempSendBlock, resp.ContractResult.Code, resp.ContractResult.Message, blockInfo)
}

func testGetLastConfigBlock(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) {
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Println("====================get last config block===================")
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   fieldWithRWSet,
			Value: []byte("true"),
		},
	}

	payloadBytes := common.ConstructQueryPayload(syscontract.SystemContract_CHAIN_QUERY.String(), "GET_LAST_CONFIG_BLOCK", pairs)

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, "", payloadBytes, nil)

	blockInfo := &commonPb.BlockInfo{}
	err := proto.Unmarshal(resp.ContractResult.Result, blockInfo)
	if err != nil {
		fmt.Printf(logTempUnmarshalBlockInfoFailed, err)
		os.Exit(0)
	}
	fmt.Printf(logTempSendBlock, resp.ContractResult.Code, resp.ContractResult.Message, blockInfo)
}

func testGetLastBlock(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) {
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Println("=======================get last block=======================")
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   fieldWithRWSet,
			Value: []byte("true"),
		},
	}

	payloadBytes := common.ConstructQueryPayload(syscontract.SystemContract_CHAIN_QUERY.String(), "GET_LAST_BLOCK", pairs)

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, "", payloadBytes, nil)

	blockInfo := &commonPb.BlockInfo{}
	err := proto.Unmarshal(resp.ContractResult.Result, blockInfo)
	if err != nil {
		fmt.Printf(logTempUnmarshalBlockInfoFailed, err)
		os.Exit(0)
	}
	fmt.Printf(logTempSendBlock, resp.ContractResult.Code, resp.ContractResult.Message)
}

func testGetChainInfo(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) {
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Println("=======================get chain info=======================")
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	// 构造Payload
	pairs := []*commonPb.KeyValuePair{}

	payload := common.ConstructQueryPayload(syscontract.SystemContract_CHAIN_QUERY.String(), "GET_CHAIN_INFO", pairs)

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, "", payload, nil)

	chainInfo := &discoveryPb.ChainInfo{}
	err := proto.Unmarshal(resp.ContractResult.Result, chainInfo)
	if err != nil {
		fmt.Printf("chainInfo unmarshal error %s\n", err)
		os.Exit(0)
	}
	fmt.Printf(logTempSendBlock, resp.ContractResult.Code, resp.ContractResult.Message, chainInfo)
}

func testCreate(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) {
	common.CreateContract(sk3, client, chainId, contractName, WasmPath, runtimeType)
}

func testUpgrade(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) {
	fmt.Println("============================================================")
	fmt.Println("============================================================")
	fmt.Println("========================test upgrade========================")
	fmt.Println("============================================================")
	fmt.Println("============================================================")

	resp := common.UpgradeContract(sk3, client, chainId, contractName, WasmUpgradePath, runtimeType)

	fmt.Printf(logTempSendTx, resp.Code, resp.Message, resp.ContractResult)
}

var fileHash = "b4018d181b6f"

func testUpgradeInvokeSum(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) string {
	txId := utils.GetRandTxId()
	fmt.Printf("\n============ invoke contract %s[sum][%s] ============\n", contractName, txId)

	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "arg1",
			Value: []byte("1"),
		},
		{
			Key:   "arg2",
			Value: []byte("2"),
		},
	}
	payload := &commonPb.Payload{
		ContractName: contractName,
		Method:       "sum",
		Parameters:   pairs,
	}

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, txId, payload, nil)

	fmt.Printf(logTempSendTx, resp.Code, resp.Message, resp.ContractResult)
	return txId
}
func testInvokeFactSave(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) string {
	txId := utils.GetRandTxId()
	fmt.Printf("\n============ invoke contract %s[save] [%s] ============\n", contractName, txId)

	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "file_hash",
			Value: []byte(fileHash),
		},
		{
			Key:   "time",
			Value: []byte("1615188470000"),
		},
		{
			Key:   "file_name",
			Value: []byte("长安链chainmaker"),
		},
	}
	payload := &commonPb.Payload{
		ContractName: contractName,
		Method:       "save",
		Parameters:   pairs,
	}

	//payloadBytes, err := proto.Marshal(payload)
	//if err != nil {
	//	log.Fatalf(logTempMarshalPayLoadFailed, err.Error())
	//}

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_INVOKE_CONTRACT,
		chainId, txId, payload, nil)

	fmt.Printf(logTempSendTx, resp.Code, resp.Message, resp.ContractResult)
	return txId
}

func testInvokeMethod(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, method string) string {
	txId := utils.GetRandTxId()
	fmt.Printf("\n============ invoke contract %s[%s] [%s] ============\n", contractName, method, txId)

	// 构造Payload
	pairs := make([]*commonPb.KeyValuePair, 0)
	payload := &commonPb.Payload{
		ContractName: contractName,
		Method:       method,
		Parameters:   pairs,
	}

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_INVOKE_CONTRACT,
		CHAIN1, txId, payload, nil)

	fmt.Printf(logTempSendTx, resp.Code, resp.Message, resp.ContractResult)
	return txId
}
func testQueryMethod(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, method string) []byte {
	txId := utils.GetRandTxId()
	fmt.Printf("\n============ invoke contract %s[%s] [%s] ============\n", contractName, method, txId)

	// 构造Payload
	pairs := make([]*commonPb.KeyValuePair, 0)
	payload := &commonPb.Payload{
		ContractName: contractName,
		Method:       method,
		Parameters:   pairs,
	}

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		CHAIN1, txId, payload, nil)

	fmt.Printf(logTempSendTx, resp.Code, resp.Message, resp.ContractResult)
	return resp.ContractResult.Result
}

func testInvokeFunctionalVerify(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) string {
	txId := utils.GetRandTxId()
	fmt.Printf("\n============ invoke contract %s[functional_verify] [%s] [functional_verify] ============\n", contractName, txId)

	// 构造Payload
	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "contract_name",
			Value: []byte(contractName),
		},
	}
	payload := &commonPb.Payload{
		ContractName: contractName,
		Method:       "functional_verify",
		Parameters:   pairs,
	}

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_INVOKE_CONTRACT,
		chainId, txId, payload, nil)

	fmt.Printf(logTempSendTx, resp.Code, resp.Message, resp.ContractResult)
	return txId
}

func testQueryFindByHash(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) (string, []byte) {
	txId := utils.GetRandTxId()
	fmt.Printf("\n============ query contract %s[find_by_file_hash] fileHash=%s ============\n", contractName, fileHash)

	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "file_hash",
			Value: []byte(fileHash),
		},
	}

	payload := &commonPb.Payload{
		ContractName: contractName,
		Method:       "find_by_file_hash",
		Parameters:   pairs,
	}

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, txId, payload, nil)

	fmt.Printf(logTempSendTx, resp.Code, resp.Message, resp.ContractResult)
	fmt.Println(string(resp.ContractResult.Result))
	//items := serialize.EasyUnmarshal(resp.ContractResult.Result)
	//for _, item := range items {
	//	fmt.Println(item.Key, item.Value)
	//}
	return txId, resp.ContractResult.Result
}

//
//func common.ProposalRequest(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, txType commonPb.TxType,
//	chainId, txId string, payload *commonPb.Payload) *commonPb.TxResponse {
//
//	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Duration(5*time.Second)))
//	defer cancel()
//
//	if txId == "" {
//		txId = utils.GetRandTxId()
//	}
//
//	file, err := ioutil.ReadFile(userCrtPath)
//	if err != nil {
//		panic(err)
//	}
//
//	// 构造Sender
//	//pubKeyString, _ := sk3.PublicKey().String()
//	sender := &acPb.Member{
//		OrgId:      orgId,
//		MemberInfo: file,
//		//IsFullCert: true,
//		//MemberInfo: []byte(pubKeyString),
//	}
//
//	// 构造Header
//	header := &commonPb.Payload{
//		ChainId: chainId,
//		//Sender:         sender,
//		TxType:         txType,
//		TxId:           txId,
//		Timestamp:      time.Now().Unix(),
//		ExpirationTime: 0,
//	}
//
//	req := &commonPb.TxRequest{
//		Payload: header,
//		Sender:  &commonPb.EndorsementEntry{Signer: sender},
//	}
//
//	// 拼接后，计算Hash，对hash计算签名
//	rawTxBytes, err := utils.CalcUnsignedTxRequestBytes(req)
//	if err != nil {
//		log.Fatalf("CalcUnsignedTxRequest failed, %s", err.Error())
//		os.Exit(0)
//	}
//
//	fmt.Errorf("################ %s", string(sender.MemberInfo))
//
//	signer := getSigner(sk3, sender)
//	//signBytes, err := signer.Sign("SHA256", rawTxBytes)
//	signBytes, err := signer.Sign("SM3", rawTxBytes)
//	if err != nil {
//		log.Fatalf("sign failed, %s", err.Error())
//		os.Exit(0)
//	}
//
//	req.Sender.Signature = signBytes
//
//	result, err := (*client).SendRequest(ctx, req)
//
//	if err != nil {
//		statusErr, ok := status.FromError(err)
//		if ok && statusErr.Code() == codes.DeadlineExceeded {
//			fmt.Println("WARN: client.call err: deadline")
//			os.Exit(0)
//		}
//		fmt.Printf("ERROR: client.call err: %v\n", err)
//		os.Exit(0)
//	}
//	return result
//}

func getSigner(sk3 crypto.PrivateKey, sender *acPb.Member) protocol.SigningMember {
	skPEM, err := sk3.String()
	if err != nil {
		log.Fatalf("get sk PEM failed, %s", err.Error())
	}
	//fmt.Printf("skPEM: %s\n", skPEM)

	m, err := accesscontrol.MockAccessControl().NewMemberFromCertPem(sender.OrgId, string(sender.MemberInfo))
	if err != nil {
		panic(err)
	}

	signer, err := accesscontrol.MockAccessControl().NewSigningMember(m, skPEM, "")
	if err != nil {
		panic(err)
	}
	return signer
}

func initGRPCConnect(useTLS bool) (*grpc.ClientConn, error) {
	url := fmt.Sprintf("%s:%d", IP, Port)

	if useTLS {
		tlsClient := ca.CAClient{
			ServerName: "chainmaker.org",
			CaPaths:    caPaths,
			CertFile:   userCrtPath,
			KeyFile:    userKeyPath,
		}

		c, err := tlsClient.GetCredentialsByCA()
		if err != nil {
			log.Fatalf("GetTLSCredentialsByCA err: %v", err)
			return nil, err
		}
		return grpc.Dial(url, grpc.WithTransportCredentials(*c))
	} else {
		return grpc.Dial(url, grpc.WithInsecure())
	}
}

//func acSign(msg *commonPb.Payload, orgIdList []int) ([]*commonPb.EndorsementEntry, error) {
//	msg.Endorsement = nil
//	bytes, _ := proto.Marshal(msg)
//
//	signers := make([]protocol.SigningMember, 0)
//	for _, orgId := range orgIdList {
//
//		numStr := strconv.Itoa(orgId)
//		path := fmt.Sprintf(prePathFmt, numStr) + "admin1.sign.key"
//		file, err := ioutil.ReadFile(path)
//		if err != nil {
//			panic(err)
//		}
//		sk, err := asym.PrivateKeyFromPEM(file, nil)
//		if err != nil {
//			panic(err)
//		}
//
//		userCrtPath := fmt.Sprintf(prePathFmt, numStr) + "admin1.sign.crt"
//		file2, err := ioutil.ReadFile(userCrtPath)
//		//fmt.Println("node", orgId, "crt", string(file2))
//		if err != nil {
//			panic(err)
//		}
//
//		// 获取peerId
//		_, err = helper.GetLibp2pPeerIdFromCert(file2)
//		//fmt.Println("node", orgId, "peerId", peerId)
//
//		// 构造Sender
//		sender1 := &acPb.Member{
//			OrgId:      "wx-org" + numStr + ".chainmaker.org",
//			MemberInfo: file2,
//			//IsFullCert: true,
//		}
//
//		signer := getSigner(sk, sender1)
//		signers = append(signers, signer)
//	}
//
//	return accesscontrol.MockSignWithMultipleNodes(bytes, signers, "SHA256")
//}

func testWaitTx(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string, txId string) {
	fmt.Printf("\n============ testWaitTx [%s] ============\n", txId)
	// 构造Payload
	pair := &commonPb.KeyValuePair{Key: "txId", Value: []byte(txId)}
	var pairs []*commonPb.KeyValuePair
	pairs = append(pairs, pair)

	payloadBytes := common.ConstructQueryPayload(syscontract.SystemContract_CHAIN_QUERY.String(), "GET_TX_BY_TX_ID", pairs)

	resp := common.ProposalRequest(sk3, client, commonPb.TxType_QUERY_CONTRACT,
		chainId, txId, payloadBytes, nil)
	if resp == nil || resp.ContractResult == nil || strings.Contains(resp.Message, "no such transaction") {
		time.Sleep(time.Second * 2)
		testWaitTx(sk3, client, chainId, txId)
	} else if resp != nil && len(resp.Message) != 0 {
		fmt.Println(resp.Message)
	}
}
