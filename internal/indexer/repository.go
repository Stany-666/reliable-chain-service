package indexer

import (
	"context"
	"errors"
	"reliable-chain-service/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	DB *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		DB: db,
	}
}

func (r *Repository) Begin(ctx context.Context) (pgx.Tx, error) {
	return r.DB.Begin(ctx)
}

func (r *Repository) InsertLog(
	ctx context.Context,
	tx pgx.Tx,
	chainID uint64,
	contractAddress string,
	blockNumber uint64,
	txHash string,
	logIndex uint,
) error {
	_, err := tx.Exec(
		ctx,
		`
		INSERT INTO indexed_logs (

		chain_id,
		contract_address,
		block_number,
		tx_hash,
		log_index
	)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (chain_id, tx_hash, log_index)
	DO NOTHING
		`,
		chainID,
		contractAddress,
		blockNumber,
		txHash,
		logIndex,
	)
	if err != nil {

		return err
	}
	return nil
}

func (r *Repository) LoadCheckpoint(
	ctx context.Context,
	name string,
) (uint64, bool, error) {
	var block uint64

	err := r.DB.QueryRow(
		ctx,

		`
		SELECT last_processed_block
		FROM scanner_checkpoints
		WHERE name = $1
		`,
		name,
	).Scan(&block)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}

		return 0, false, err
	}

	return block, true, nil
}

func (r *Repository) SaveCheckpoint(
	ctx context.Context,
	tx pgx.Tx,
	name string,
	block uint64,
) error {
	_, err := tx.Exec(
		ctx,
		`
	INSERT INTO scanner_checkpoints (
		name,
		last_processed_block,
		updated_at
	)
	VALUES ($1, $2, NOW())

	ON CONFLICT (name)
	DO UPDATE SET
		last_processed_block = EXCLUDED.last_processed_block,
		updated_at = NOW()
	`,
		name,
		block,
	)

	return err
}

func (r *Repository) InsertTransferEvent(
	ctx context.Context,
	tx pgx.Tx,
	event *model.TransferEvent,
) error {
	_, err := tx.Exec(
		ctx,
		`
		INSERT INTO transfer_events (
			tx_hash,
			log_index,
			block_number,
			token_address,
			from_address,
			to_address,
			amount,
			block_hash
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (tx_hash, log_index)
		DO NOTHING
		`,
		event.TxHash,
		event.LogIndex,
		event.BlockNumber,
		event.TokenAddress.Hex(),
		event.From.Hex(),
		event.To.Hex(),
		event.Amount.String(),
		event.BlockHash,
	)
	return err
}
