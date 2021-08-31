/*
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badgerdbprovider

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"chainmaker.org/chainmaker-go/localconf"
	"chainmaker.org/chainmaker/protocol/v2"
	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/options"
	"github.com/pkg/errors"
)

const (
	defaultCompression    = 0
	defaultValueThreshold = 1024 * 10
)

const (
	//StoreBlockDBDir blockdb folder name
	StoreBlockDBDir = "store_block"
	//StoreStateDBDir statedb folder name
	StoreStateDBDir = "store_state"
	//StoreHistoryDBDir historydb folder name
	StoreHistoryDBDir = "store_history"
	//StoreResultDBDir resultdb folder name
	StoreResultDBDir = "store_result"
)

// BadgerDBHandle encapsulated handle to badgerdb
type BadgerDBHandle struct {
	writeLock sync.Mutex
	db        *badger.DB
	logger    protocol.Logger
}

func NewBadgerDBHandle(chainId string, dbFolder string, dbconfig *localconf.BadgerDbConfig,
	logger protocol.Logger) *BadgerDBHandle {
	dbPath := filepath.Join(dbconfig.StorePath, chainId, dbFolder)
	opt := badger.DefaultOptions(dbPath)
	opt.SyncWrites = false
	opt.Compression = defaultCompression
	opt.ValueThreshold = defaultValueThreshold

	if dbconfig.Compression != 0 && dbconfig.Compression < 3 {
		opt.Compression = options.CompressionType(dbconfig.Compression)
	}
	if dbconfig.ValueThreshold > 0 {
		opt.ValueThreshold = dbconfig.ValueThreshold
	}

	err := createDirIfNotExist(dbPath)
	if err != nil {
		panic(fmt.Sprintf("Error create dir %s by badgerdbprovider: %s", dbPath, err))
	}
	db, err := badger.Open(opt)
	if err != nil {
		panic(fmt.Sprintf("Error opening %s by badgerdbprovider: %s", dbPath, err))
	}
	logger.Debugf("open badgerdb:%s", dbPath)
	return &BadgerDBHandle{
		db:     db,
		logger: logger,
	}
}

func createDirIfNotExist(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		// 创建文件夹
		err := os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
}

// Get returns the value for the given key, or returns nil if none exists
func (h *BadgerDBHandle) Get(key []byte) ([]byte, error) {
	var value []byte
	err := h.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		value, err = item.ValueCopy(nil)
		return err
	})

	if err == badger.ErrKeyNotFound {
		value = nil
		err = nil
	} else if err != nil {
		h.logger.Errorf("getting badgerdbprovider key [%#v], err:%s", key, err.Error())
		return nil, errors.Wrapf(err, "error getting badgerdbprovider key [%#v]", key)
	}
	return value, nil
}

// Put saves the key-values
func (h *BadgerDBHandle) Put(key []byte, value []byte) error {
	if value == nil {
		h.logger.Warn("writing badgerdbprovider key [%#v] with nil value", key)
		return errors.New("error writing badgerdbprovider with nil value")
	}
	wb := h.db.NewWriteBatch()
	err := wb.Set(key, value)
	if err != nil {
		return err
	}
	err = wb.Flush()
	if err != nil {
		h.logger.Errorf("writing badgerdbprovider key [%#v]", key)
		return errors.Wrapf(err, "error writing badgerdbprovider key [%#v]", key)
	}
	return err
}

// Has return true if the given key exist, or return false if none exists
func (h *BadgerDBHandle) Has(key []byte) (bool, error) {
	exist := false
	err := h.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err != nil {
			return err
		}
		exist = true
		return nil
	})

	if err == badger.ErrKeyNotFound {
		exist = false
		err = nil
	} else if err != nil {
		h.logger.Errorf("getting badgerdbprovider key [%#v], err:%s", key, err.Error())
		return false, errors.Wrapf(err, "error getting badgerdbprovider key [%#v]", key)
	}
	return exist, nil
}

// Delete deletes the given key
func (h *BadgerDBHandle) Delete(key []byte) error {
	wb := h.db.NewWriteBatch()
	defer wb.Cancel()
	err := wb.Delete(key)
	if err != nil {
		h.logger.Errorf("deleting badgerdbprovider key [%#v]", key)
		return errors.Wrapf(err, "error deleting badgerdbprovider key [%#v]", key)
	}
	return err
}

// WriteBatch writes a batch in an atomic operation
//nolint:golint,unused
func (h *BadgerDBHandle) WriteBatch(batch protocol.StoreBatcher, sync bool) error {
	if batch.Len() == 0 {
		return nil
	}
	h.writeLock.Lock()
	defer h.writeLock.Unlock()
	badgerBatch := h.db.NewWriteBatch()
	for k, v := range batch.KVs() {
		key := []byte(k)
		if v == nil {
			_ = badgerBatch.Delete(key)
		} else {
			_ = badgerBatch.Set(key, v)
		}
	}

	if err := badgerBatch.Flush(); err != nil {
		h.logger.Errorf("write batch to badgerdb provider failed")
		return errors.Wrap(err, "error writing batch to badgerdb provider")
	}
	return nil
}

// CompactRange compacts the underlying DB for the given key range.
func (h *BadgerDBHandle) CompactRange(start, limit []byte) error { //nolint:golint,unused
	return nil
	//return h.db.CompactRange(util.Range{
	//	Start: start,
	//	Limit: limit,
	//})
}

// NewIteratorWithRange returns an iterator that contains all the key-values between given key ranges
// start is included in the results and limit is excluded.
func (h *BadgerDBHandle) NewIteratorWithRange(startKey []byte, limitKey []byte) (protocol.Iterator, error) {
	if len(startKey) == 0 || len(limitKey) == 0 {
		return nil, fmt.Errorf("iterator range should not start(%s) or limit(%s) with empty key",
			string(startKey), string(limitKey))
	}
	return NewIterator(h.db, badger.DefaultIteratorOptions, startKey, limitKey), nil
}

// NewIteratorWithPrefix returns an iterator that contains all the key-values with given prefix
func (h *BadgerDBHandle) NewIteratorWithPrefix(prefix []byte) (protocol.Iterator, error) {
	if len(prefix) == 0 {
		return nil, fmt.Errorf("iterator prefix should not be empty key")
	}

	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	return NewIterator(h.db, opts, nil, nil), nil
}

// Close closes the badgerdb
func (h *BadgerDBHandle) Close() error {
	h.writeLock.Lock()
	defer h.writeLock.Unlock()
	return h.db.Close()
}
