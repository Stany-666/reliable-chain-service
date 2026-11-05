package scanner

import (
	"context"
	"fmt"
	"math/big"
	"reliable-chain-service/internal/ethereum"
	"reliable-chain-service/internal/indexer"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

func RunLive(
	ctx context.Context,
	client *ethereum.Client,
	repo *indexer.Repository,
	contractAddress common.Address,
	confirmationDepth uint64,
	initialLookback uint64,
	batchSize uint64,
) error {

	latestBlock, err := client.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}

	safeLast := latestBlock - confirmationDepth

	checkpoint, found, err := repo.LoadCheckpoint(ctx, "usdt-mainnet-live")
	if err != nil {
		return err
	}

	var start uint64
	var transferTopic = common.HexToHash(
		"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
	)

	if found {
		start = checkpoint + 1
	} else {
		start = safeLast - initialLookback + 1
	}

	for batchStart := start; batchStart <= safeLast; batchStart += batchSize {
		batchEnd := batchStart + batchSize - 1

		if batchEnd > safeLast {
			batchEnd = safeLast
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

		err = processBatch(
			ctx,
			repo,
			"usdt-mainnet-live",
			logs,
			batchEnd,
		)
		if err != nil {
			return err
		}

		fmt.Println("checkpoint saved:", batchEnd)
	}
	return nil
}
