/*
 * Copyright (C) BABEC. All rights reserved.
 * Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package contractmgr

import (
	"encoding/json"
	"fmt"
	"regexp"

	"chainmaker.org/chainmaker-go/vm/native/common"

	"chainmaker.org/chainmaker/pb-go/syscontract"

	"chainmaker.org/chainmaker-go/utils"

	"chainmaker.org/chainmaker-go/logger"
	commonPb "chainmaker.org/chainmaker/pb-go/common"
	"chainmaker.org/chainmaker/protocol"
)

var (
	ContractName = syscontract.SystemContract_CONTRACT_MANAGE.String()
)

type ContractManager struct {
	methods map[string]common.ContractFunc
	log     *logger.CMLogger
}

func NewContractManager(log *logger.CMLogger) *ContractManager {
	return &ContractManager{
		log:     log,
		methods: registerContractManagerMethods(log),
	}
}

func (c *ContractManager) GetMethod(methodName string) common.ContractFunc {
	return c.methods[methodName]
}

func registerContractManagerMethods(log *logger.CMLogger) map[string]common.ContractFunc {
	methodMap := make(map[string]common.ContractFunc, 64)
	runtime := &ContractManagerRuntime{log: log}
	methodMap[syscontract.ContractManageFunction_INIT_CONTRACT.String()] = runtime.installContract
	methodMap[syscontract.ContractManageFunction_UPGRADE_CONTRACT.String()] = runtime.upgradeContract
	methodMap[syscontract.ContractManageFunction_FREEZE_CONTRACT.String()] = runtime.freezeContract
	methodMap[syscontract.ContractManageFunction_UNFREEZE_CONTRACT.String()] = runtime.unfreezeContract
	methodMap[syscontract.ContractManageFunction_REVOKE_CONTRACT.String()] = runtime.revokeContract
	methodMap[syscontract.ContractQueryFunction_GET_CONTRACT_INFO.String()] = runtime.getContractInfo
	return methodMap

}
func (r *ContractManagerRuntime) getContractInfo(txSimContext protocol.TxSimContext, parameters map[string][]byte) ([]byte, error) {
	name := string(parameters[syscontract.GetContractInfo_CONTRACT_NAME.String()])
	contract, err := r.GetContractInfo(txSimContext, name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(contract)
}
func (r *ContractManagerRuntime) getAllContracts(txSimContext protocol.TxSimContext, parameters map[string][]byte) ([]byte, error) {
	contracts, err := r.GetAllContracts(txSimContext)
	if err != nil {
		return nil, err
	}
	return json.Marshal(contracts)
}
func (r *ContractManagerRuntime) installContract(txSimContext protocol.TxSimContext, parameters map[string][]byte) ([]byte, error) {
	name := string(parameters[syscontract.InitContract_CONTRACT_NAME.String()])
	version := string(parameters[syscontract.InitContract_CONTRACT_VERSION.String()])
	byteCode := parameters[syscontract.InitContract_CONTRACT_BYTECODE.String()]
	runtime := parameters[syscontract.InitContract_CONTRACT_RUNTIME_TYPE.String()]
	runtimeInt := commonPb.RuntimeType_value[string(runtime)]
	runtimeType := commonPb.RuntimeType(runtimeInt)
	contract, err := r.InstallContract(txSimContext, name, version, byteCode, runtimeType, parameters)
	if err != nil {
		return nil, err
	}
	return contract.Marshal()
}
func (r *ContractManagerRuntime) upgradeContract(txSimContext protocol.TxSimContext, parameters map[string][]byte) ([]byte, error) {
	name := string(parameters[syscontract.UpgradeContract_CONTRACT_NAME.String()])
	version := string(parameters[syscontract.UpgradeContract_CONTRACT_VERSION.String()])
	byteCode := parameters[syscontract.UpgradeContract_CONTRACT_BYTECODE.String()]
	runtime := string(parameters[syscontract.UpgradeContract_CONTRACT_RUNTIME_TYPE.String()])
	runtimeInt := commonPb.RuntimeType_value[runtime]
	runtimeType := commonPb.RuntimeType(runtimeInt)
	contract, err := r.UpgradeContract(txSimContext, name, version, byteCode, runtimeType, parameters)
	if err != nil {
		return nil, err
	}
	return contract.Marshal()
}
func (r *ContractManagerRuntime) freezeContract(txSimContext protocol.TxSimContext, parameters map[string][]byte) ([]byte, error) {
	name := string(parameters[syscontract.GetContractInfo_CONTRACT_NAME.String()])
	contract, err := r.FreezeContract(txSimContext, name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(contract)
}
func (r *ContractManagerRuntime) unfreezeContract(txSimContext protocol.TxSimContext, parameters map[string][]byte) ([]byte, error) {
	name := string(parameters[syscontract.GetContractInfo_CONTRACT_NAME.String()])
	contract, err := r.UnfreezeContract(txSimContext, name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(contract)
}
func (r *ContractManagerRuntime) revokeContract(txSimContext protocol.TxSimContext, parameters map[string][]byte) ([]byte, error) {
	name := string(parameters[syscontract.GetContractInfo_CONTRACT_NAME.String()])
	contract, err := r.RevokeContract(txSimContext, name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(contract)
}

type ContractManagerRuntime struct {
	log *logger.CMLogger
}

//GetContractInfo 根据合约名字查询合约的详细信息
func (r *ContractManagerRuntime) GetContractInfo(context protocol.TxSimContext, name string) (*commonPb.Contract, error) {
	if utils.IsAnyBlank(name) {
		err := fmt.Errorf("%s, param[contract_name] of get contract not found", common.ErrParams.Error())
		r.log.Errorf(err.Error())
		return nil, err
	}
	return utils.GetContractByName(context.Get, name)
}
func (r *ContractManagerRuntime) GetContractByteCode(context protocol.TxSimContext, name string) ([]byte, error) {
	if utils.IsAnyBlank(name) {
		err := fmt.Errorf("%s, param[contract_name] of get contract not found", common.ErrParams.Error())
		r.log.Errorf(err.Error())
		return nil, err
	}
	return utils.GetContractBytecode(context.Get, name)
}

//GetAllContracts 查询所有合约的详细信息
func (r *ContractManagerRuntime) GetAllContracts(context protocol.TxSimContext) ([]*commonPb.Contract, error) {
	keyPrefix := []byte(utils.PrefixContractInfo)
	it, err := context.Select(syscontract.SystemContract_CONTRACT_MANAGE.String(), keyPrefix, keyPrefix)
	if err != nil {
		return nil, err
	}
	defer it.Release()
	var result []*commonPb.Contract
	for it.Next() {
		contract := &commonPb.Contract{}
		kv, err := it.Value()
		if err != nil {
			return nil, err
		}
		err = contract.Unmarshal(kv.Value)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

//安装新合约
func (r *ContractManagerRuntime) InstallContract(context protocol.TxSimContext, name, version string, byteCode []byte,
	runTime commonPb.RuntimeType, initParameters map[string][]byte) (*commonPb.Contract, error) {
	if !checkContractName(name) {
		return nil, errInvalidContractName
	}
	if runTime == commonPb.RuntimeType_EVM && !checkEvmAddress(name) {
		return nil, errInvalidEvmContractName
	}
	key := utils.GetContractDbKey(name)
	//check name exist
	existContract, _ := context.Get(ContractName, key)
	if len(existContract) > 0 { //exist
		return nil, errContractExist
	}
	contract := &commonPb.Contract{
		Name:        name,
		Version:     version,
		RuntimeType: runTime,
		Status:      commonPb.ContractStatus_NORMAL,
		Creator:     context.GetSender(),
	}
	cdata, _ := contract.Marshal()

	context.Put(ContractName, key, cdata)
	byteCodeKey := utils.GetContractByteCodeDbKey(name)
	context.Put(ContractName, byteCodeKey, byteCode)
	//实例化合约，并init合约，产生读写集
	result, statusCode := context.CallContract(contract, protocol.ContractInitMethod, byteCode, initParameters, 0, commonPb.TxType_INVOKE_CONTRACT)
	if statusCode != commonPb.TxStatusCode_SUCCESS {
		return nil, errContractInitFail
	}
	if result.Code > 0 { //throw error
		return nil, errContractInitFail
	}
	if runTime == commonPb.RuntimeType_EVM {
		//save bytecode body
		//EVM的特殊处理，在调用构造函数后会返回真正需要存的字节码，这里将之前的字节码覆盖
		if len(result.Result) > 0 {
			err := context.Put(ContractName, byteCodeKey, result.Result)
			if err != nil {
				return nil, errContractInitFail
			}
		}
	}
	return contract, nil
}

//升级现有合约
func (r *ContractManagerRuntime) UpgradeContract(context protocol.TxSimContext, name, version string, byteCode []byte,
	runTime commonPb.RuntimeType, upgradeParameters map[string][]byte) (*commonPb.Contract, error) {
	key := utils.GetContractDbKey(name)
	//check name exist
	existContract, _ := context.Get(ContractName, key)
	if len(existContract) == 0 { //not exist
		return nil, errContractNotExist
	}
	contract := &commonPb.Contract{}
	err := contract.Unmarshal(existContract)
	if err != nil {
		return nil, err
	}
	if contract.Version == version {
		return nil, errContractVersionExist
	}
	contract.RuntimeType = runTime
	//update ContractInfo
	cdata, _ := contract.Marshal()
	context.Put(ContractName, key, cdata)
	//update Contract Bytecode
	byteCodeKey := utils.GetContractByteCodeDbKey(name)
	context.Put(ContractName, byteCodeKey, byteCode)
	//运行新合约的upgrade方法，产生读写集
	result, statusCode := context.CallContract(contract, protocol.ContractUpgradeMethod, byteCode, upgradeParameters, 0, commonPb.TxType_INVOKE_CONTRACT)
	if statusCode != commonPb.TxStatusCode_SUCCESS {
		return nil, errContractUpgradeFail
	}
	if result.Code > 0 { //throw error
		return nil, errContractUpgradeFail
	}
	if runTime == commonPb.RuntimeType_EVM {
		//save bytecode body
		//EVM的特殊处理，在调用构造函数后会返回真正需要存的字节码，这里将之前的字节码覆盖
		if len(result.Result) > 0 {
			err := context.Put(ContractName, byteCodeKey, result.Result)
			if err != nil {
				return nil, errContractUpgradeFail
			}
		}
	}
	return contract, nil
}
func (r *ContractManagerRuntime) FreezeContract(context protocol.TxSimContext, name string) (*commonPb.Contract, error) {
	return r.changeContractStatus(context, name, commonPb.ContractStatus_NORMAL, commonPb.ContractStatus_FROZEN)
}
func (r *ContractManagerRuntime) UnfreezeContract(context protocol.TxSimContext, name string) (*commonPb.Contract, error) {
	return r.changeContractStatus(context, name, commonPb.ContractStatus_FROZEN, commonPb.ContractStatus_NORMAL)
}
func (r *ContractManagerRuntime) RevokeContract(context protocol.TxSimContext, name string) (*commonPb.Contract, error) {
	return r.changeContractStatus(context, name, commonPb.ContractStatus_NORMAL, commonPb.ContractStatus_REVOKED)
}

func (r *ContractManagerRuntime) changeContractStatus(context protocol.TxSimContext, name string, oldStatus, newStatus commonPb.ContractStatus) (*commonPb.Contract, error) {
	if utils.IsAnyBlank(name) {
		err := fmt.Errorf("%s, param[contract_name] of get contract not found", common.ErrParams.Error())
		r.log.Errorf(err.Error())
		return nil, err
	}
	contract, err := utils.GetContractByName(context.Get, name)
	if err != nil {
		return nil, err
	}
	if contract.Status != oldStatus {
		return nil, errContractStatusInvalid
	}
	contract.Status = newStatus
	key := utils.GetContractDbKey(name)
	cdata, _ := contract.Marshal()
	err = context.Put(ContractName, key, cdata)
	if err != nil {
		return nil, err
	}
	return contract, nil
}
func checkContractName(name string) bool {
	reg := regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_]{0,127}$")
	return reg.Match([]byte(name))
}
func checkEvmAddress(addr string) bool {
	reg := regexp.MustCompile("^(0x)?[0-9a-fA-F]{40}$")
	return reg.Match([]byte(addr))
}
