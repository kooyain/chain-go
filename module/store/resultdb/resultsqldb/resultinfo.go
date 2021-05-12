/*
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resultsqldb

import (
	commonpb "chainmaker.org/chainmaker-go/pb/protogo/common"
	"github.com/gogo/protobuf/proto"
)

// StateHistoryInfo defines mysql orm model, used to create mysql table 'result_infos'
type ResultInfo struct {
	TxId        string `gorm:"size:128;primaryKey"`
	BlockHeight int64
	TxIndex     int
	Rwset       []byte `gorm:"type:longblob"`
	Status      int    `gorm:"default:0"`
	Result      []byte `gorm:"type:blob"`
	Message     string `gorm:"type:longtext"`
}

func (b *ResultInfo) ScanObject(scan func(dest ...interface{}) error) error {
	return scan(&b.TxId, &b.BlockHeight, &b.TxIndex, &b.Rwset, &b.Status, &b.Result, &b.Message)
}
func (b *ResultInfo) GetCreateTableSql(dbType string) string {
	if dbType == "mysql" {
		return "CREATE TABLE `result_infos` (`tx_id` varchar(128),`block_height` bigint,`tx_index` bigint,`rwset` longblob,`status` bigint DEFAULT 0,`result` blob,`message` longtext,PRIMARY KEY (`tx_id`))"
	} else if dbType == "sqlite" {
		return "CREATE TABLE `result_infos` (`tx_id` text,`block_height` integer,`tx_index` integer,`rwset` longblob,`status` integer DEFAULT 0,`result` blob,`message` longtext,PRIMARY KEY (`tx_id`))"
	}
	panic("Unsupported db type:" + string(dbType))
}
func (b *ResultInfo) GetTableName() string {
	return "result_infos"
}
func (b *ResultInfo) GetInsertSql() (string, []interface{}) {
	return "INSERT INTO result_infos values(?,?,?,?,?,?,?)",
		[]interface{}{b.TxId, b.BlockHeight, b.TxIndex, b.Rwset, b.Status, b.Result, b.Message}
}
func (b *ResultInfo) GetUpdateSql() (string, []interface{}) {
	return "UPDATE result_infos set block_height=?,tx_index=?,rwset=?,status=?,result=?,message=?" +
			" WHERE tx_id=?",
		[]interface{}{b.BlockHeight, b.TxIndex, b.Rwset, b.Status, b.Result, b.Message, b.TxId}
}

// NewHistoryInfo construct a new HistoryInfo
func NewResultInfo(txid string, blockHeight int64, txIndex int, result *commonpb.ContractResult, rw *commonpb.TxRWSet) *ResultInfo {
	rwBytes, _ := proto.Marshal(rw)

	return &ResultInfo{
		TxId:        txid,
		BlockHeight: blockHeight,
		TxIndex:     txIndex,
		Status:      int(result.Code),
		Result:      result.Result,
		Message:     result.Message,
		Rwset:       rwBytes,
	}
}