// Copyright (C) BABEC. All rights reserved.
// Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

package archive

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gosuri/uiprogress"
	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"chainmaker.org/chainmaker-go/tools/cmc/archive/db/mysql"
	"chainmaker.org/chainmaker-go/tools/cmc/archive/model"
	"chainmaker.org/chainmaker-go/tools/cmc/util"
	sdk "chainmaker.org/chainmaker-sdk-go"
	"chainmaker.org/chainmaker-sdk-go/pb/protogo/common"
	"chainmaker.org/chainmaker-sdk-go/pb/protogo/store"
)

const (
	// default 20 blocks per batch
	blocksPerBatch = 20
	// Send Archive Block Request timeout
	archiveBlockRequestTimeout = 20 // 20s
)

func newDumpCMD() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump",
		Short: "dump blockchain data",
		Long:  "dump blockchain data to off-chain storage and delete on-chain data",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbType != "mysql" {
				return fmt.Errorf("unsupport database type %s", dbType)
			}

			if height, err := strconv.ParseInt(target, 10, 64); err == nil {
				return runDumpByHeightCMD(height)
			} else if t, err := time.Parse("2006-01-02", target); err == nil {
				height, err := calcTargetHeightByTime(t)
				if err != nil {
					return err
				}
				return runDumpByHeightCMD(height)
			} else {
				return errors.New("invalid --target, eg. 100 (block height) or 1999-02-01 (date)")
			}
		},
	}

	attachFlags(cmd, []string{
		flagSdkConfPath, flagChainId, flagDbDest, flagTarget, flagBlocks, flagSecretKey,
	})

	cmd.MarkFlagRequired(flagSdkConfPath)
	cmd.MarkFlagRequired(flagChainId)
	cmd.MarkFlagRequired(flagDbDest)
	cmd.MarkFlagRequired(flagTarget)
	cmd.MarkFlagRequired(flagBlocks)
	cmd.MarkFlagRequired(flagSecretKey)

	return cmd
}

// runDumpByHeightCMD `dump` command implementation
func runDumpByHeightCMD(targetBlkHeight int64) error {
	//// 1.Chain Client
	cc, err := util.CreateChainClientWithSDKConf(sdkConfPath)
	if err != nil {
		return err
	}
	defer cc.Stop()

	//// 2.Database
	db, err := initDb()
	if err != nil {
		return err
	}
	locker := mysql.NewDbLocker(db, "cmc", mysql.DefaultLockLeaseAge)
	locker.Lock()
	defer locker.UnLock()

	//// 3.Validation, block height etc.
	archivedBlkHeightOnChain, err := cc.GetArchivedBlockHeight()
	if err != nil {
		return err
	}
	archivedBlkHeightOffChain, err := model.GetArchivedBlockHeight(db)
	if err != nil {
		return err
	}
	currentBlkHeightOnChain, err := cc.GetCurrentBlockHeight()
	if err != nil {
		return err
	}

	err = validateDump(archivedBlkHeightOnChain, archivedBlkHeightOffChain, currentBlkHeightOnChain, targetBlkHeight)
	if err != nil {
		return err
	}

	//// 4.Store & Archive Blocks
	var barCount = targetBlkHeight - archivedBlkHeightOnChain
	if blocks < barCount {
		barCount = blocks
	}
	bar := uiprogress.AddBar(int(barCount)).AppendCompleted().PrependElapsed()
	bar.PrependFunc(func(b *uiprogress.Bar) string {
		return fmt.Sprintf("\nArchiving Blocks (%d/%d)", b.Current(), barCount)
	})
	uiprogress.Start()
	var batchStartBlkHeight, batchEndBlkHeight = archivedBlkHeightOnChain + 1, archivedBlkHeightOnChain + 1
	if archivedBlkHeightOnChain == 0 {
		batchStartBlkHeight = 0
	}
	for processedBlocks := int64(0); targetBlkHeight >= batchEndBlkHeight && processedBlocks <= blocks; processedBlocks++ {
		if batchEndBlkHeight-batchStartBlkHeight >= blocksPerBatch {
			if err := runBatch(cc, db, batchStartBlkHeight, batchEndBlkHeight); err != nil {
				return err
			}

			batchStartBlkHeight = batchEndBlkHeight
		}

		batchEndBlkHeight++
		bar.Incr()
	}
	uiprogress.Stop()
	// do the rest of blocks
	return runBatch(cc, db, batchStartBlkHeight, batchEndBlkHeight)
}

// validateDump basic params validation
func validateDump(archivedBlkHeightOnChain, archivedBlkHeightOffChain, currentBlkHeightOnChain, targetBlkHeight int64) error {
	// target block height already archived, do nothing.
	if targetBlkHeight <= archivedBlkHeightOffChain {
		return errors.New("target block height already archived")
	}

	// required archived block height off-chain == archived block height on-chain
	if archivedBlkHeightOffChain != archivedBlkHeightOnChain {
		return errors.New("required archived block height off-chain == archived block height on-chain")
	}

	// required current block height >= target block height
	if currentBlkHeightOnChain < targetBlkHeight {
		return errors.New("required current block height >= target block height")
	}
	return nil
}

// batchGetFullBlocks Get full blocks start from startBlk end at endBlk.
// NOTE: Include startBlk, exclude endBlk
func batchGetFullBlocks(cc *sdk.ChainClient, startBlk, endBlk int64) ([]*store.BlockWithRWSet, error) {
	var blkWithRWSetSlice []*store.BlockWithRWSet
	for blk := startBlk; blk < endBlk; blk++ {
		blkWithRWSet, err := cc.GetFullBlockByHeight(blk)
		if err != nil {
			return nil, err
		}
		blkWithRWSetSlice = append(blkWithRWSetSlice, blkWithRWSet)
	}
	return blkWithRWSetSlice, nil
}

// batchStoreAndArchiveBlocks Store blocks to off-chain storage then archive blocks on-chain
func batchStoreAndArchiveBlocks(cc *sdk.ChainClient, db *gorm.DB, blkWithRWSetSlice []*store.BlockWithRWSet) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()

	// store blocks
	for _, blkWithRWSet := range blkWithRWSetSlice {
		blkWithRWSetBytes, err := blkWithRWSet.Marshal()
		if err != nil {
			return err
		}

		blkHeightBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(blkHeightBytes, uint64(blkWithRWSet.Block.Header.BlockHeight))

		sum, err := util.Hmac([]byte(chainId), blkHeightBytes, blkWithRWSetBytes, []byte(secretKey))
		if err != nil {
			return err
		}

		_, err = model.InsertBlockInfo(tx, chainId, blkWithRWSet.Block.Header.BlockHeight, blkWithRWSetBytes, sum)
		if err != nil {
			return err
		}
	}

	// archive blocks on-chain
	archivedBlkHeightOnChain := blkWithRWSetSlice[len(blkWithRWSetSlice)-1].Block.Header.BlockHeight
	err := archiveBlockOnChain(cc, archivedBlkHeightOnChain)
	if err != nil {
		return err
	}

	// update archived block height off-chain
	err = model.UpdateArchivedBlockHeight(tx, archivedBlkHeightOnChain)
	if err != nil {
		return err
	}

	return tx.Commit().Error
}

// runBatch Run a batch job
func runBatch(cc *sdk.ChainClient, db *gorm.DB, startBlk, endBlk int64) error {
	// check if create table

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()

	// get & store blocks
	for blk := startBlk; blk < endBlk; blk++ {
		var bInfo model.BlockInfo
		err := db.Table(model.BlockInfoTableNameByBlockHeight(blk)).Where("Fblock_height = ?", blk).First(&bInfo).Error
		if err == nil { // this block info was already in database, just update Fis_archived to 1
			if !bInfo.IsArchived {
				bInfo.IsArchived = true
				tx.Table(model.BlockInfoTableNameByBlockHeight(blk)).Save(&bInfo)
			}
		} else if err == gorm.ErrRecordNotFound {
			blkWithRWSet, err := cc.GetFullBlockByHeight(blk)
			if err != nil {
				return err
			}

			blkWithRWSetBytes, err := blkWithRWSet.Marshal()
			if err != nil {
				return err
			}

			blkHeightBytes := make([]byte, 8)
			binary.LittleEndian.PutUint64(blkHeightBytes, uint64(blkWithRWSet.Block.Header.BlockHeight))

			sum, err := util.Hmac([]byte(chainId), blkHeightBytes, blkWithRWSetBytes, []byte(secretKey))
			if err != nil {
				return err
			}

			_, err = model.InsertBlockInfo(tx, chainId, blkWithRWSet.Block.Header.BlockHeight, blkWithRWSetBytes, sum)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// archive blocks on-chain
	err := archiveBlockOnChain(cc, endBlk-1)
	if err != nil {
		return err
	}

	// update archived block height off-chain
	err = model.UpdateArchivedBlockHeight(tx, endBlk-1)
	if err != nil {
		return err
	}

	return tx.Commit().Error
}

// archiveBlockOnChain Build & Sign & Send a ArchiveBlockRequest
func archiveBlockOnChain(cc *sdk.ChainClient, height int64) error {
	var (
		err                error
		payload            []byte
		signedPayloadBytes []byte
		resp               *common.TxResponse
	)

	payload, err = cc.CreateArchiveBlockPayload(height)
	if err != nil {
		return err
	}

	signedPayloadBytes, err = cc.SignArchivePayload(payload)
	if err != nil {
		return err
	}

	resp, err = cc.SendArchiveBlockRequest(signedPayloadBytes, archiveBlockRequestTimeout)
	if err != nil {
		return err
	}

	return util.CheckProposalRequestResp(resp, false)
}

func calcTargetHeightByTime(t time.Time) (int64, error) {
	targetTs := t.Unix()
	cc, err := util.CreateChainClientWithSDKConf(sdkConfPath)
	if err != nil {
		return -1, err
	}
	defer cc.Stop()

	lastBlock, err := cc.GetLastBlock(false)
	if err != nil {
		return -1, err
	}
	if lastBlock.Block.Header.BlockTimestamp <= targetTs {
		return lastBlock.Block.Header.BlockHeight, nil
	}

	genesisHeader, err := cc.GetBlockHeaderByHeight(0)
	if err != nil {
		return -1, err
	}
	if genesisHeader.BlockTimestamp >= targetTs {
		return -1, fmt.Errorf("no blocks at %s", t)
	}

	return util.SearchInt64(lastBlock.Block.Header.BlockHeight, func(i int64) (bool, error) {
		header, err := cc.GetBlockHeaderByHeight(i)
		if err != nil {
			return false, err
		}
		return header.BlockTimestamp < targetTs, nil
	})
}
