package scanner

import (
	"context"
	"fmt"
	"reliable-chain-service/internal/indexer"

	"github.com/ethereum/go-ethereum/core/types"
)

func processBatch(
	ctx context.Context,
	repo *indexer.Repository,
	scannerName string,
	logs []types.Log,
	batchEnd uint64,
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

	err = repo.SaveCheckpoint(
		ctx,
		tx,
		scannerName,
		batchEnd)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)

}
