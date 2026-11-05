package scanner

import (
	"context"
	"fmt"
	"math/big"

	"reliable-chain-service/internal/ethereum"
	"reliable-chain-service/internal/indexer"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func processTransferLogs(
	ctx context.Context,
	repo *indexer.Repository,
	logs []types.Log,
) error {
	tx, err := repo.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx)

	for _, log := range logs {

		transfer, err := DecodeTransfer(log)
		if err != nil {
			return err
		}

		err = repo.InsertLog(
			ctx,
			tx,
			1,
			log.Address.Hex(),
			log.BlockNumber,
			log.TxHash.Hex(),
			log.Index,
		)
		if err != nil {
			return err
		}

		err = repo.InsertTransferEvent(
			ctx,
			tx,
			transfer,
		)
		if err != nil {
			return err
		}

		fmt.Println("indexed log", log.TxHash.Hex(), log.Index)

	}
	return tx.Commit(ctx)

}

func RunEventReplay(
	ctx context.Context,
	client *ethereum.Client,
	repo *indexer.Repository,
	contractAddress common.Address,
	batchSize uint64,
) error {

	var transferTopic = common.HexToHash(
		"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
	)

	start := uint64(25488872)
	end := uint64(25488882)

	for batchStart := start; batchStart <= end; batchStart += batchSize {
		batchEnd := batchStart + batchSize - 1

		if batchEnd > end {
			batchEnd = end
		}

		query := geth.FilterQuery{
			FromBlock: new(big.Int).SetUint64(batchStart),
			ToBlock:   new(big.Int).SetUint64(batchEnd),
			Addresses: []common.Address{
				contractAddress,
			},
			Topics: [][]common.Hash{
				{
					transferTopic,
				},
			},
		}

		logs, err := client.RPC.FilterLogs(ctx, query)
		if err != nil {
			return err
		}

		err = processTransferLogs(
			ctx,
			repo,
			logs,
		)
		if err != nil {
			return err
		}

		fmt.Println("processed block range:", batchStart, "-", batchEnd)
	}
	return nil
}
