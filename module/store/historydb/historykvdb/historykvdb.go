/*
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package historykvdb

import (
	"chainmaker.org/chainmaker-go/protocol"
	"chainmaker.org/chainmaker-go/store/cache"
	"chainmaker.org/chainmaker-go/store/historydb"
	"chainmaker.org/chainmaker-go/store/serialization"
	"chainmaker.org/chainmaker-go/store/types"
	"encoding/binary"
	"fmt"
)

const (
	historyDBName           = ""
	keyHistoryPrefix        = "k"
	accountTxHistoryPrefix  = "a"
	contractTxHistoryPrefix = "c"
	historyDBSavepointKey   = "historySavepointKey"
)

// HistoryKvDB provider a implementation of `historydb.HistoryDB`
// This implementation provides a key-value based data model
type HistoryKvDB struct {
	dbHandle protocol.DBHandle
	cache    *cache.StoreCacheMgr
	logger   protocol.Logger
}

func NewHistoryKvDB(db protocol.DBHandle, cache *cache.StoreCacheMgr, log protocol.Logger) *HistoryKvDB {
	return &HistoryKvDB{
		dbHandle: db,
		cache:    cache,
		logger:   log,
	}
}

func (h *HistoryKvDB) InitGenesis(genesisBlock *serialization.BlockWithSerializedInfo) error {
	return h.CommitBlock(genesisBlock)
}

// CommitBlock commits the block rwsets in an atomic operation
func (h *HistoryKvDB) CommitBlock(blockInfo *serialization.BlockWithSerializedInfo) error {
	batch := types.NewUpdateBatch()
	// 1. last block height
	block := blockInfo.Block
	lastBlockNumBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(lastBlockNumBytes, uint64(block.Header.BlockHeight))
	batch.Put([]byte(historyDBSavepointKey), lastBlockNumBytes)
	blockHeight := uint64(block.Header.BlockHeight)
	txRWSets := blockInfo.TxRWSets
	for _, txRWSet := range txRWSets {
		txId := txRWSet.TxId
		for _, write := range txRWSet.TxWrites {
			key := constructKey(write.ContractName, write.Key, blockHeight, txId)
			batch.Put(key, []byte{}) //write key modify history
		}
	}
	for _, tx := range block.Txs {
		accountId := tx.GetSenderAccountId()
		txId := tx.Header.TxId
		contractName, err := tx.GetContractName()
		if err != nil {
			h.logger.Errorf("get contract name fail from tx[%s],err:%s", tx.Header.TxId, err.Error())
			continue
		}
		batch.Put(constructAcctTxHistKey(accountId, blockHeight, txId), []byte{})
		batch.Put(constructContractTxHistKey(contractName, blockHeight, txId), []byte{})
	}
	err := h.writeBatch(block.Header.BlockHeight, batch)
	if err != nil {
		return err
	}
	h.logger.Debugf("chain[%s]: commit history block[%d]",
		block.Header.ChainId, block.Header.BlockHeight)
	return nil
}

// GetLastSavepoint returns the last block height
func (h *HistoryKvDB) GetLastSavepoint() (uint64, error) {
	bytes, err := h.get([]byte(historyDBSavepointKey))
	if err != nil {
		return 0, err
	} else if bytes == nil {
		return 0, nil
	}
	num := binary.BigEndian.Uint64(bytes)
	return num, nil
}

// Close is used to close database
func (h *HistoryKvDB) Close() {
	h.logger.Info("close history kv db")
	h.dbHandle.Close()
}

func (h *HistoryKvDB) writeBatch(blockHeight int64, batch protocol.StoreBatcher) error {
	//update cache
	h.cache.AddBlock(blockHeight, batch)
	go func() {
		err := h.dbHandle.WriteBatch(batch, false)
		if err != nil {
			panic(fmt.Sprintf("Error writting db: %s", err))
		}
		//db committed, clean cache
		h.cache.DelBlock(blockHeight)
	}()
	return nil
}

func (h *HistoryKvDB) get(key []byte) ([]byte, error) {
	//get from cache
	value, exist := h.cache.Get(string(key))
	if exist {
		return value, nil
	}
	//get from database
	return h.dbHandle.Get(key)
}

func (h *HistoryKvDB) has(key []byte) (bool, error) {
	//check has from cache
	isDelete, exist := h.cache.Has(string(key))
	if exist {
		return !isDelete, nil
	}
	return h.dbHandle.Has(key)
}

type historyKeyIterator struct {
	dbIter    protocol.Iterator
	buildFunc func(key []byte) (*historydb.BlockHeightTxId, error)
}

func (i *historyKeyIterator) Next() bool {
	return i.dbIter.Next()
}
func (i *historyKeyIterator) Value() (*historydb.BlockHeightTxId, error) {
	err := i.dbIter.Error()
	if err != nil {
		return nil, err
	}
	return i.buildFunc(i.dbIter.Key())
}
func (i *historyKeyIterator) Release() {
	i.dbIter.Release()
}
