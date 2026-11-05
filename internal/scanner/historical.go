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

func RunHistorical(
	ctx context.Context,
	client *ethereum.Client,
	repo *indexer.Repository,
	contractAddress common.Address,
	end uint64,
	batchSize uint64,
) error {

	checkpoint, found, err := repo.LoadCheckpoint(ctx, "historical-mainnet")
	if err != nil {
		return err
	}

	if !found {
		return fmt.Errorf("checkpoint not found")
	}

	start := checkpoint + 1

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
		}

		logs, err := client.RPC.FilterLogs(ctx, query)
		if err != nil {
			return err
		}

		err = processBatch(
			ctx,
			repo,
			"historical-mainnet",
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
