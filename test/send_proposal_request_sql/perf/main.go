/*
Copyright (C) BABEC. All rights reserved.
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"chainmaker.org/chainmaker-go/accesscontrol"
	"chainmaker.org/chainmaker-go/common/ca"
	"chainmaker.org/chainmaker-go/common/crypto"
	"chainmaker.org/chainmaker-go/common/crypto/asym"
	"chainmaker.org/chainmaker-go/common/helper"
	acPb "chainmaker.org/chainmaker-go/pb/protogo/accesscontrol"
	apiPb "chainmaker.org/chainmaker-go/pb/protogo/api"
	commonPb "chainmaker.org/chainmaker-go/pb/protogo/common"
	"chainmaker.org/chainmaker-go/protocol"
	"chainmaker.org/chainmaker-go/utils"
	"context"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

const (
	logTempMarshalPayLoadFailed = "marshal payload failed, %s"
	logTempSendTx               = "send tx resp: code:%d, msg:%s, payload:%+v\n"
)

const (
	CHAIN1         = "chain1"
	IP             = "localhost"
	Port           = 12351
	certPathPrefix = "../../../config-sql"
	userKeyPath    = certPathPrefix + "/crypto-config/wx-org1.chainmaker.org/user/client1/client1.tls.key"
	userCrtPath    = certPathPrefix + "/crypto-config/wx-org1.chainmaker.org/user/client1/client1.tls.crt"
	orgId          = "wx-org1.chainmaker.org"
	prePathFmt     = certPathPrefix + "/crypto-config/wx-org%s.chainmaker.org/user/admin1/"
)

var (
	WasmPath        = ""
	WasmUpgradePath = ""
	contractName    = ""
	runtimeType     = commonPb.RuntimeType_WASMER
)

var caPaths = []string{certPathPrefix + "/crypto-config/wx-org1.chainmaker.org/ca"}

func main() {

	conn, err := initGRPCConnect(true)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	client := apiPb.NewRpcNodeClient(conn)

	file, err := ioutil.ReadFile(userKeyPath)
	if err != nil {
		panic(err)
	}

	sk3, err := asym.PrivateKeyFromPEM(file, nil)
	if err != nil {
		panic(err)
	}

	// test
	fmt.Println("\n\n\n\n======wasmer test=====\n\n\n\n")
	initWasmerSqlTest()

	performanceTestCreate(sk3, &client)

	performanceTestUpdate(sk3, &client)
	time.Sleep(4 * time.Second)

	performanceTestBlank(sk3, &client)
	time.Sleep(4 * time.Second)

	performanceTestInsert(sk3, &client)
}

func other(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient) {
	txId := "9e36eaedfcbe43a792fb516b1d2c9adb49049f247a044399ab5accc61cc7d880"
	code := testGetTxByTxId(sk3, client, txId, CHAIN1)
	if code == commonPb.ContractResultCode_FAIL {
		fmt.Println("查询失败")
	} else {
		fmt.Println("查询成功")
	}
}

var count = 30000
var goroutineNumber = 5

func performanceTestInsert(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient) {
	fmt.Println("执行插入一条数据")
	txId := ""
	txPreId := ""
	start := utils.CurrentTimeMillisSeconds()
	// 2) 执行合约-sql insert
	totalCount := count * goroutineNumber
	wg := sync.WaitGroup{}
	wg.Add(goroutineNumber)
	for j := 0; j < goroutineNumber; j++ {
		go func() {
			for i := 0; i < count; i++ {
				txPreId = txId
				txId = testInvokeSqlInsert(sk3, client, CHAIN1, strconv.Itoa(i))
				time.Sleep(time.Millisecond * 4)
				if i%(count/10) == 0 {
					fmt.Println(runtime.NumGoroutine(), "this goroutine count =", i, "/", totalCount)
				}
			}
			wg.Done()
		}()
	}
	wg.Wait()
	time.Sleep(time.Second * 4)
	// wait
	for {
		code := testGetTxByTxId(sk3, client, txId, CHAIN1)
		code2 := testGetTxByTxId(sk3, client, txPreId, CHAIN1)
		if code == commonPb.ContractResultCode_OK && code2 == commonPb.ContractResultCode_OK {
			break
		}
		time.Sleep(time.Millisecond * 2000)
	}
	end1 := utils.CurrentTimeMillisSeconds()
	fmt.Println("time cost \t", end1-start, "  start", start, "  end", end1, "  count", totalCount)
	fmt.Println("tps \t", int64(totalCount*1000)/(end1-start))
}
func performanceTestBlank(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient) {

	fmt.Println("执行空方法")
	txId := ""
	txPreId := ""
	start := utils.CurrentTimeMillisSeconds()
	// 2) 执行合约-sql insert
	totalCount := count * goroutineNumber
	wg := sync.WaitGroup{}
	wg.Add(goroutineNumber)
	for j := 0; j < goroutineNumber; j++ {
		go func() {
			for i := 0; i < count; i++ {
				txPreId = txId
				txId = testInvokeSqlBlank(sk3, client, CHAIN1, strconv.Itoa(i))
				time.Sleep(time.Millisecond * 4)
				if i%(count/10) == 0 {
					fmt.Println(runtime.NumGoroutine(), "this goroutine count =", i, "/", totalCount)
				}
			}
			wg.Done()
		}()
	}
	wg.Wait()
	time.Sleep(time.Second * 4)
	// wait
	for {
		code := testGetTxByTxId(sk3, client, txId, CHAIN1)
		code2 := testGetTxByTxId(sk3, client, txPreId, CHAIN1)
		if code == commonPb.ContractResultCode_OK && code2 == commonPb.ContractResultCode_OK {
			break
		}
		time.Sleep(time.Millisecond * 2000)
	}
	end1 := utils.CurrentTimeMillisSeconds()
	fmt.Println("time cost \t", end1-start, "  start", start, "  end", end1, "  count", totalCount)
	fmt.Println("tps \t", int64(totalCount*1000)/(end1-start))
}
func performanceTestUpdate(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient) {

	txId := ""
	txPreId := ""
	fmt.Println("插入一条，再跟进ID更新")
	txId = testInvokeSqlInsert(sk3, client, CHAIN1, "1")
	time.Sleep(4 * time.Second)

	start := utils.CurrentTimeMillisSeconds()
	// 2) 执行合约-sql insert
	totalCount := count * goroutineNumber
	wg := sync.WaitGroup{}
	wg.Add(goroutineNumber)
	for j := 0; j < goroutineNumber; j++ {
		go func() {
			for i := 0; i < count; i++ {
				txPreId = txId
				testInvokeSqlUpdate(sk3, client, CHAIN1, strconv.Itoa(i), txId)
				time.Sleep(time.Millisecond * 4)
				if i%(count/10) == 0 {
					fmt.Println(runtime.NumGoroutine(), "this goroutine count =", i, "/", totalCount)
				}
			}
			wg.Done()
		}()
	}
	wg.Wait()
	time.Sleep(time.Second * 4)
	// wait
	for {
		code := testGetTxByTxId(sk3, client, txId, CHAIN1)
		code2 := testGetTxByTxId(sk3, client, txPreId, CHAIN1)
		if code == commonPb.ContractResultCode_OK && code2 == commonPb.ContractResultCode_OK {
			break
		}
		time.Sleep(time.Millisecond * 2000)
	}
	end1 := utils.CurrentTimeMillisSeconds()
	fmt.Println("time cost \t", end1-start, "  start", start, "  end", end1, "  count", totalCount)
	fmt.Println("tps \t", int64(totalCount*1000)/(end1-start))
}
func performanceTestCreate(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient) {

	// 1) 合约创建
	testCreate(sk3, client, CHAIN1)
	time.Sleep(4 * time.Second)

}
func initWasmerSqlTest() {
	WasmPath = "rust-sql-perf-1.1.0.wasm"
	WasmUpgradePath = "rust-sql-perf-1.1.0.wasm"
	contractName = "contract10002"
	runtimeType = commonPb.RuntimeType_WASMER
}
func initGasmSqlTest() {
	WasmPath = "../../wasm/go-sql-1.1.0.wasm"
	WasmUpgradePath = "../../wasm/go-sql-1.1.0.wasm"
	contractName = "contract2001"
	runtimeType = commonPb.RuntimeType_GASM
}
func initWxwmSqlTest() {
	WasmPath = "../../wasm/cpp-func-verify-1.0.0.wasm"
	WasmUpgradePath = "../../wasm/cpp-func-verify-1.0.0.wasm"
	contractName = "contract300"
	runtimeType = commonPb.RuntimeType_WXVM
}
func testCreate(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string) {

	txId := utils.GetRandTxId()

	fmt.Printf("\n============ create contract %s [%s] ============\n", contractName, txId)

	//wasmBin, _ := base64.StdEncoding.DecodeString(WasmPath)
	wasmBin, _ := ioutil.ReadFile(WasmPath)
	var pairs []*commonPb.KeyValuePair

	method := commonPb.ManageUserContractFunction_INIT_CONTRACT.String()

	payload := &commonPb.ContractMgmtPayload{
		ChainId: chainId,
		ContractId: &commonPb.ContractId{
			ContractName:    contractName,
			ContractVersion: "1.0.0",
			//RuntimeType:     commonPb.RuntimeType_GASM,
			RuntimeType: runtimeType,
		},
		Method:     method,
		Parameters: pairs,
		ByteCode:   wasmBin,
	}

	if endorsement, err := acSign(payload, []int{1, 2, 3, 4}); err == nil {
		payload.Endorsement = endorsement
	} else {
		log.Fatalf("testCreate failed to sign endorsement, %s", err.Error())
		os.Exit(0)
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		log.Fatalf(logTempMarshalPayLoadFailed, err.Error())
		os.Exit(0)
	}

	resp := proposalRequest(sk3, client, commonPb.TxType_MANAGE_USER_CONTRACT,
		chainId, txId, payloadBytes)

	fmt.Printf(logTempSendTx, resp.Code, resp.Message, resp.ContractResult)
	if resp.Code != 0 {
		panic(resp.Message)
	}
}

func testInvokeSqlInsert(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string, number string) string {
	txId := utils.GetRandTxId()

	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "id",
			Value: txId,
		},
		{
			Key:   "number",
			Value: number,
		},
	}
	payload := &commonPb.TransactPayload{
		ContractName: contractName,
		Method:       "sql_insert",
		Parameters:   pairs,
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		log.Fatalf(logTempMarshalPayLoadFailed, err.Error())
	}

	proposalRequest(sk3, client, commonPb.TxType_INVOKE_USER_CONTRACT,
		chainId, txId, payloadBytes)

	return txId
}

func testInvokeSqlUpdate(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string, number string, txId string) string {
	// 构造Payload
	pairs := []*commonPb.KeyValuePair{
		{
			Key:   "id",
			Value: txId,
		},
		{
			Key:   "number",
			Value: number,
		},
	}
	payload := &commonPb.TransactPayload{
		ContractName: contractName,
		Method:       "sql_update",
		Parameters:   pairs,
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		log.Fatalf(logTempMarshalPayLoadFailed, err.Error())
	}

	txId = utils.GetRandTxId()
	proposalRequest(sk3, client, commonPb.TxType_INVOKE_USER_CONTRACT,
		chainId, txId, payloadBytes)

	return txId
}

func testInvokeSqlBlank(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, chainId string, number string) string {
	txId := utils.GetRandTxId()

	// 构造Payload
	pairs := make([]*commonPb.KeyValuePair, 0)
	payload := &commonPb.TransactPayload{
		ContractName: contractName,
		Method:       "sql_blank",
		Parameters:   pairs,
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		log.Fatalf(logTempMarshalPayLoadFailed, err.Error())
	}

	proposalRequest(sk3, client, commonPb.TxType_INVOKE_USER_CONTRACT,
		chainId, txId, payloadBytes)

	return txId
}

func proposalRequest(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, txType commonPb.TxType,
	chainId, txId string, payloadBytes []byte) *commonPb.TxResponse {

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	defer cancel()

	if txId == "" {
		txId = utils.GetRandTxId()
	}

	file, err := ioutil.ReadFile(userCrtPath)
	if err != nil {
		panic(err)
	}

	// 构造Sender
	//pubKeyString, _ := sk3.PublicKey().String()
	sender := &acPb.SerializedMember{
		OrgId:      orgId,
		MemberInfo: file,
		IsFullCert: true,
		//MemberInfo: []byte(pubKeyString),
	}

	// 构造Header
	header := &commonPb.TxHeader{
		ChainId:        chainId,
		Sender:         sender,
		TxType:         txType,
		TxId:           txId,
		Timestamp:      time.Now().Unix(),
		ExpirationTime: 0,
	}

	req := &commonPb.TxRequest{
		Header:    header,
		Payload:   payloadBytes,
		Signature: nil,
	}

	// 拼接后，计算Hash，对hash计算签名
	rawTxBytes, err := utils.CalcUnsignedTxRequestBytes(req)
	if err != nil {
		log.Fatalf("CalcUnsignedTxRequest failed, %s", err.Error())
		os.Exit(0)
	}

	signer := getSigner(sk3, sender)
	//signBytes, err := signer.Sign("SHA256", rawTxBytes)
	signBytes, err := signer.Sign("SM3", rawTxBytes)
	if err != nil {
		log.Fatalf("sign failed, %s", err.Error())
		os.Exit(0)
	}

	req.Signature = signBytes

	result, err := (*client).SendRequest(ctx, req)

	if err != nil {
		statusErr, ok := status.FromError(err)
		if ok && statusErr.Code() == codes.DeadlineExceeded {
			fmt.Println("WARN: client.call err: deadline")
			os.Exit(0)
		}
		fmt.Printf("ERROR: client.call err: %v\n", err)
		os.Exit(0)
	}
	return result
}

func getSigner(sk3 crypto.PrivateKey, sender *acPb.SerializedMember) protocol.SigningMember {
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

func constructPayload(contractName, method string, pairs []*commonPb.KeyValuePair) []byte {
	payload := &commonPb.QueryPayload{
		ContractName: contractName,
		Method:       method,
		Parameters:   pairs,
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		log.Fatalf(logTempMarshalPayLoadFailed, err.Error())
		os.Exit(0)
	}

	return payloadBytes
}

func acSign(msg *commonPb.ContractMgmtPayload, orgIdList []int) ([]*commonPb.EndorsementEntry, error) {
	msg.Endorsement = nil
	bytes, _ := proto.Marshal(msg)

	signers := make([]protocol.SigningMember, 0)
	for _, orgId := range orgIdList {

		numStr := strconv.Itoa(orgId)
		path := fmt.Sprintf(prePathFmt, numStr) + "admin1.sign.key"
		file, err := ioutil.ReadFile(path)
		if err != nil {
			panic(err)
		}
		sk, err := asym.PrivateKeyFromPEM(file, nil)
		if err != nil {
			panic(err)
		}

		userCrtPath := fmt.Sprintf(prePathFmt, numStr) + "admin1.sign.crt"
		file2, err := ioutil.ReadFile(userCrtPath)
		if err != nil {
			panic(err)
		}

		// 获取peerId
		_, err = helper.GetLibp2pPeerIdFromCert(file2)

		// 构造Sender
		sender1 := &acPb.SerializedMember{
			OrgId:      "wx-org" + numStr + ".chainmaker.org",
			MemberInfo: file2,
			IsFullCert: true,
		}

		signer := getSigner(sk, sender1)
		signers = append(signers, signer)
	}

	return accesscontrol.MockSignWithMultipleNodes(bytes, signers, "SHA256")
}
func panicNotEqual(a string, b string) {
	if a != b {
		panic(a + " not equal " + b)
	}
}

func testGetTxByTxId(sk3 crypto.PrivateKey, client *apiPb.RpcNodeClient, txId, chainId string) commonPb.ContractResultCode {
	//now := time.Now()
	//fmt.Printf("\n%d-%d-%dT %d:%d:%d============ get tx by txId [%s] ============\n", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), txId)

	// 构造Payload
	pair := &commonPb.KeyValuePair{Key: "txId", Value: txId}
	var pairs []*commonPb.KeyValuePair
	pairs = append(pairs, pair)

	payloadBytes := constructPayload(commonPb.ContractName_SYSTEM_CONTRACT_QUERY.String(), "GET_TX_BY_TX_ID", pairs)

	resp := proposalRequest(sk3, client, commonPb.TxType_QUERY_SYSTEM_CONTRACT,
		chainId, txId, payloadBytes)

	result := &commonPb.TransactionInfo{}
	err := proto.Unmarshal(resp.ContractResult.Result, result)
	if err != nil {
		fmt.Println(err)
		return commonPb.ContractResultCode_FAIL
	}
	if result.Transaction == nil {
		return commonPb.ContractResultCode_FAIL
	}
	if result.Transaction.Result == nil {
		return commonPb.ContractResultCode_FAIL
	}
	if result.Transaction.Result.ContractResult == nil {
		return commonPb.ContractResultCode_FAIL
	}
	return result.Transaction.Result.ContractResult.Code
}
