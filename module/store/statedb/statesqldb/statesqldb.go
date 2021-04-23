/*
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statesqldb

import (
	"chainmaker.org/chainmaker-go/localconf"
	logImpl "chainmaker.org/chainmaker-go/logger"
	commonPb "chainmaker.org/chainmaker-go/pb/protogo/common"
	storePb "chainmaker.org/chainmaker-go/pb/protogo/store"
	"chainmaker.org/chainmaker-go/protocol"
	"chainmaker.org/chainmaker-go/store/dbprovider/sqldbprovider"
	"sync"
)

// StateSqlDB provider a implementation of `statedb.StateDB`
// This implementation provides a mysql based data model
type StateSqlDB struct {
	db      protocol.SqlDBHandle
	logger  protocol.Logger
	chainId string
	sync.Mutex
}

//如果数据库不存在，则创建数据库，然后切换到这个数据库，创建表
//如果数据库存在，则切换数据库，检查表是否存在，不存在则创建表。
func (db *StateSqlDB) initDb(dbName string) {
	db.logger.Debugf("try to create state db %s", dbName)
	err := db.db.CreateDatabaseIfNotExist(dbName)
	if err != nil {
		panic("init state sql db fail")
	}
	db.logger.Debug("try to create state db table: state_infos")
	err = db.db.CreateTableIfNotExist(&StateInfo{})
	if err != nil {
		panic("init state sql db table fail")
	}
}

// NewStateMysqlDB construct a new `StateDB` for given chainId
func NewStateSqlDB(chainId string, dbConfig *localconf.SqlDbConfig, logger protocol.Logger) (*StateSqlDB, error) {
	db := sqldbprovider.NewSqlDBHandle(chainId, dbConfig, logger)
	return newStateSqlDB(chainId, db, logger)
}

func newStateSqlDB(chainId string, db protocol.SqlDBHandle, logger protocol.Logger) (*StateSqlDB, error) {
	if logger == nil {
		logger = logImpl.GetLoggerByChain(logImpl.MODULE_STORAGE, chainId)
	}
	stateDB := &StateSqlDB{
		db:      db,
		logger:  logger,
		chainId: chainId,
	}

	return stateDB, nil
}
func (s *StateSqlDB) InitGenesis(genesisBlock *storePb.BlockWithRWSet) error {
	s.Lock()
	defer s.Unlock()
	s.initDb(getDbName(genesisBlock.Block.Header.ChainId))
	return s.commitBlock(genesisBlock)
}
func getDbName(chainId string) string {
	return "statedb_" + chainId
}

func GetContractDbName(chainId, contractName string) string {
	if _, ok := commonPb.ContractName_value[contractName]; ok { //如果是系统合约，不为每个合约构建数据库，使用统一个statedb数据库
		return getDbName(chainId)
	}
	return "statedb_" + chainId + "_" + contractName
}

// CommitBlock commits the state in an atomic operation
func (s *StateSqlDB) CommitBlock(blockWithRWSet *storePb.BlockWithRWSet) error {
	s.Lock()
	defer s.Unlock()
	return s.commitBlock(blockWithRWSet)
}
func (s *StateSqlDB) commitBlock(blockWithRWSet *storePb.BlockWithRWSet) error {
	block := blockWithRWSet.Block
	txRWSets := blockWithRWSet.TxRWSets
	txKey := block.GetTxKey()
	if len(txRWSets) == 0 {
		s.logger.Warnf("block[%d] don't have any read write set data", block.Header.BlockHeight)
		return nil
	}
	dbTx, err := s.db.GetDbTransaction(txKey)
	s.logger.Infof("GetDbTransaction db:%v,err:%s", dbTx, err)
	processStateDbSqlOutside := false
	if err == nil { //外部已经开启了事务，状态数据在外部提交
		s.logger.Debugf("db transaction[%s] already created outside, don't need process statedb sql in CommitBlock function", txKey)
		processStateDbSqlOutside = true
	}
	//没有在外部开启事务，则开启事务，进行数据写入
	if !processStateDbSqlOutside {
		dbTx, err = s.db.BeginDbTransaction(txKey)
		if err != nil {
			return err
		}
	}
	if block.IsContractMgmtBlock() {
		//创建对应合约的数据库
		payload := &commonPb.ContractMgmtPayload{}
		payload.Unmarshal(block.Txs[0].RequestPayload)
		dbName := GetContractDbName(block.Header.ChainId, payload.ContractId.ContractName)
		s.initDb(dbName) //创建KV表
		writes := txRWSets[0].TxWrites
		for _, txWrite := range writes {
			if len(txWrite.Key) == 0 { //这是SQL语句
				s.db.ChangeContextDb(dbName)
				_, err := s.db.ExecSql(string(txWrite.Value)) //运行用户自定义的建表语句
				if err != nil {
					s.db.RollbackDbTransaction(txKey)
					return err
				}
			} else { //是KV数据，直接存储到StateInfo表
				s.db.ChangeContextDb(GetContractDbName(block.Header.ChainId, txWrite.ContractName))
				stateInfo := NewStateInfo(txWrite.ContractName, txWrite.Key, txWrite.Value, block.Header.BlockHeight)
				if _, err := s.db.Save(stateInfo); err != nil {
					s.logger.Errorf("save state key[%s] get error:%s", txWrite.Key, err.Error())
					s.db.RollbackDbTransaction(txKey)
					return err
				}
			}
		}
		err = s.db.CommitDbTransaction(txKey)
		if err != nil {
			return err
		}
		s.logger.Debugf("chain[%s]: commit state block[%d]",
			block.Header.ChainId, block.Header.BlockHeight)
		return nil
	}

	currentDb := ""
	for _, txRWSet := range txRWSets {
		for _, txWrite := range txRWSet.TxWrites {
			contractDbName := GetContractDbName(s.chainId, txWrite.ContractName)
			if txWrite.ContractName != "" && (contractDbName != currentDb || currentDb == "") { //切换DB
				dbTx.ChangeContextDb(contractDbName)
				currentDb = contractDbName
			}
			if len(txWrite.Key) == 0 && !processStateDbSqlOutside { //是sql,而且没有在外面处理过，则在这里进行处理
				sql := string(txWrite.Value)
				if _, err := dbTx.ExecSql(sql); err != nil {
					s.logger.Errorf("execute sql[%s] get error:%s", txWrite.Value, err.Error())
					s.db.RollbackDbTransaction(txKey)
					return err
				}
			} else {
				stateInfo := NewStateInfo(txWrite.ContractName, txWrite.Key, txWrite.Value, block.Header.BlockHeight)
				if _, err := dbTx.Save(stateInfo); err != nil {
					s.logger.Errorf("save state key[%s] get error:%s", txWrite.Key, err.Error())
					s.db.RollbackDbTransaction(txKey)
					return err
				}
			}
		}
	}
	err = s.db.CommitDbTransaction(txKey)
	if err != nil {
		s.logger.Error(err.Error())
		return err
	}
	s.logger.Debugf("chain[%s]: commit state block[%d]",
		block.Header.ChainId, block.Header.BlockHeight)
	return nil
}

// ReadObject returns the state value for given contract name and key, or returns nil if none exists.
func (s *StateSqlDB) ReadObject(contractName string, key []byte) ([]byte, error) {
	s.Lock()
	defer s.Unlock()
	if contractName != "" {
		if err := s.db.ChangeContextDb(GetContractDbName(s.chainId, contractName)); err != nil {
			return nil, err
		}
	}
	sql := "select object_value from state_infos where contract_name=? and object_key=?"

	res, err := s.db.QuerySingle(sql, contractName, key)
	if err != nil {
		s.logger.Errorf("failed to read state, contract:%s, key:%s,error:%s", contractName, key, err)
		return nil, err
	}
	if res.IsEmpty() {
		s.logger.Infof(" read empty state, contract:%s, key:%s", contractName, key)
		return nil, nil
	}
	var stateValue []byte

	err = res.ScanColumns(&stateValue)
	if err != nil {
		s.logger.Errorf("failed to read state, contract:%s, key:%s", contractName, key)
		return nil, err
	}
	s.logger.Infof(" read right state, contract:%s, key:%s valLen:%d", contractName, key, len(stateValue))
	return stateValue, nil
}

// SelectObject returns an iterator that contains all the key-values between given key ranges.
// startKey is included in the results and limit is excluded.
func (s *StateSqlDB) SelectObject(contractName string, startKey []byte, limit []byte) protocol.Iterator {
	s.Lock()
	defer s.Unlock()
	if contractName != "" {
		if err := s.db.ChangeContextDb(GetContractDbName(s.chainId, contractName)); err != nil {
			return nil
		}
	}
	sql := "select * from state_infos where object_key between ? and ?"
	rows, err := s.db.QueryMulti(sql, startKey, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	result := &kvIterator{}
	for rows.Next() {
		var kv StateInfo
		rows.ScanObject(&kv)
		result.append(&kv)
	}
	return result
}

// GetLastSavepoint returns the last block height
func (s *StateSqlDB) GetLastSavepoint() (uint64, error) {
	s.Lock()
	defer s.Unlock()
	sql := "select max(block_height) from state_infos"
	row, err := s.db.QuerySingle(sql)
	if err != nil {
		return 0, err
	}
	var height *uint64
	err = row.ScanColumns(&height)
	if err != nil {
		return 0, err
	}
	if height == nil {
		return 0, nil
	}
	return *height, nil
}

// Close is used to close database, there is no need for gorm to close db
func (s *StateSqlDB) Close() {
	s.Lock()
	defer s.Unlock()
	s.logger.Info("close state sql db")
	s.db.Close()
}

func (s *StateSqlDB) QuerySingle(contractName, sql string, values ...interface{}) (protocol.SqlRow, error) {
	s.Lock()
	defer s.Unlock()
	dbName := GetContractDbName(s.chainId, contractName)
	if contractName != "" {
		if err := s.db.ChangeContextDb(dbName); err != nil {
			return nil, err
		}
	}
	row, err := s.db.QuerySingle(sql, values...)
	if row.IsEmpty() {
		s.logger.Infof("query single return empty row. sql:%s,db name:%s", sql, dbName)
	}
	return row, err
}
func (s *StateSqlDB) QueryMulti(contractName, sql string, values ...interface{}) (protocol.SqlRows, error) {
	s.Lock()
	defer s.Unlock()
	if contractName != "" {
		if err := s.db.ChangeContextDb(GetContractDbName(s.chainId, contractName)); err != nil {
			return nil, err
		}
	}
	return s.db.QueryMulti(sql, values...)

}
func (s *StateSqlDB) ExecDdlSql(contractName, sql string) error {
	s.Lock()
	defer s.Unlock()
	dbName := GetContractDbName(s.chainId, contractName)
	err := s.db.CreateDatabaseIfNotExist(dbName)
	if err != nil {
		return err
	}
	s.logger.Debugf("run DDL sql[%s] in db[%s]", sql, dbName)
	_, err = s.db.ExecSql(sql)
	return err
}
func (s *StateSqlDB) BeginDbTransaction(txName string) (protocol.SqlDBTransaction, error) {
	s.Lock()
	defer s.Unlock()
	return s.db.BeginDbTransaction(txName)

}
func (s *StateSqlDB) GetDbTransaction(txName string) (protocol.SqlDBTransaction, error) {
	s.Lock()
	defer s.Unlock()
	return s.db.GetDbTransaction(txName)

}
func (s *StateSqlDB) CommitDbTransaction(txName string) error {
	s.Lock()
	defer s.Unlock()
	return s.db.CommitDbTransaction(txName)

}
func (s *StateSqlDB) RollbackDbTransaction(txName string) error {
	s.Lock()
	defer s.Unlock()
	return s.db.RollbackDbTransaction(txName)
}
