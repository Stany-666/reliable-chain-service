package asset

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	DB *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{DB: db}
}

func (r *Repository) FindAsset(
	ctx context.Context,
	chainID uint64,
	tokenAddress string,
) (Asset, bool, error) {
	var asset Asset

	err := r.DB.QueryRow(
		ctx,
		`
		SELECT id, chain_id, symbol, token_address, decimals
		FROM assets
		WHERE chain_id = $1
		  AND LOWER(token_address) = LOWER($2)
		`,
		chainID,
		tokenAddress,
	).Scan(
		&asset.ID,
		&asset.ChainID,
		&asset.Symbol,
		&asset.TokenAddress,
		&asset.Decimals,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Asset{}, false, nil
		}
		return Asset{}, false, err
	}

	return asset, true, nil
}

func (r *Repository) FindDepositAddress(
	ctx context.Context,
	chainID uint64,
	address string,
) (DepositAddress, bool, error) {
	var depositAddress DepositAddress

	err := r.DB.QueryRow(
		ctx,
		`
		SELECT user_id, chain_id, address
		FROM deposit_addresses
		WHERE chain_id = $1
		  AND LOWER(address) = LOWER($2)
		`,
		chainID,
		address,
	).Scan(
		&depositAddress.UserID,
		&depositAddress.ChainID,
		&depositAddress.Address,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DepositAddress{}, false, nil
		}
		return DepositAddress{}, false, err
	}

	return depositAddress, true, nil
}

func (r *Repository) CreateDeposit(
	ctx context.Context,
	deposit Deposit,
) (bool, error) {
	tag, err := r.DB.Exec(
		ctx,
		`
		INSERT INTO deposits (
			transfer_event_id,
			user_id,
			asset_id,
			amount,
			status
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (transfer_event_id)
		DO NOTHING
		`,
		deposit.TransferEventID,
		deposit.UserID,
		deposit.AssetID,
		deposit.Amount.String(),
		string(deposit.Status),
	)
	if err != nil {
		return false, err
	}

	return tag.RowsAffected() == 1, nil
}

func (r *Repository) LoadUnprocessedTransfers(
	ctx context.Context,
	limit int,
) ([]TransferRecord, error) {
	rows, err := r.DB.Query(
		ctx,
		`
		SELECT
			te.id,
			te.tx_hash,
			te.log_index,
			te.block_number,
			te.token_address,
			te.from_address,
			te.to_address,
			te.amount
		FROM mock_transfer_events te
		LEFT JOIN deposits d
			ON d.transfer_event_id = te.id
		WHERE d.id IS NULL
		ORDER BY te.id
		LIMIT $1
		`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transfers []TransferRecord

	for rows.Next() {
		var transfer TransferRecord
		var amountString string

		err := rows.Scan(
			&transfer.ID,
			&transfer.TxHash,
			&transfer.LogIndex,
			&transfer.BlockNumber,
			&transfer.TokenAddress,
			&transfer.FromAddress,
			&transfer.ToAddress,
			&amountString,
		)
		if err != nil {
			return nil, err
		}

		amount, ok := new(big.Int).SetString(amountString, 10)
		if !ok {
			return nil, fmt.Errorf("invalid transfer amount: %s", amountString)
		}

		transfer.Amount = amount
		transfers = append(transfers, transfer)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return transfers, nil
}

func (r *Repository) UpdateDepositStatus(
	ctx context.Context,
	depositID int64,
	status DepositStatus,
) error {
	if status != DepositStatusConfirmed && status != DepositStatusReverted {
		return fmt.Errorf("invalid deposit status: %s", status)
	}
	tag, err := r.DB.Exec(
		ctx,
		`
		UPDATE deposits
		SET status = $1,
		    updated_at = NOW()
		WHERE id = $2
		  AND status = $3
		`,
		status,
		depositID,
		DepositStatusDetected,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("deposit %d is no longer detected", depositID)
	}
	return nil
}

func (r *Repository) FindTransferEventByID(
	ctx context.Context,
	id int64,
) (TransferRecord, error) {

	var event TransferRecord
	var amountString string

	err := r.DB.QueryRow(
		ctx,
		`
		SELECT
			id,
			tx_hash,
			log_index,
			block_number,
			block_hash,
			token_address,
			from_address,
			to_address,
			amount
		FROM mock_transfer_events
		WHERE id = $1
		`,
		id,
	).Scan(
		&event.ID,
		&event.TxHash,
		&event.LogIndex,
		&event.BlockNumber,
		&event.BlockHash,
		&event.TokenAddress,
		&event.FromAddress,
		&event.ToAddress,
		&amountString,
	)

	if err != nil {
		return TransferRecord{}, err
	}

	amount, ok := new(big.Int).SetString(amountString, 10)
	if !ok {
		return TransferRecord{}, fmt.Errorf(
			"invalid transfer amount: %s",
			amountString,
		)
	}

	event.Amount = amount

	return event, nil
}

func (r *Repository) LoadPendingDeposits(
	ctx context.Context,
	limit int,
) ([]Deposit, error) {

	rows, err := r.DB.Query(
		ctx,
		`
        SELECT
            id,
            transfer_event_id,
            user_id,
            asset_id,
            amount::text,
            status
        FROM deposits
        WHERE status = $1
        LIMIT $2
        `,
		DepositStatusDetected,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deposits := make([]Deposit, 0)

	for rows.Next() {
		var deposit Deposit
		var amount string

		err := rows.Scan(
			&deposit.ID,
			&deposit.TransferEventID,
			&deposit.UserID,
			&deposit.AssetID,
			&amount,
			&deposit.Status,
		)
		if err != nil {
			return nil, err
		}

		deposit.Amount = new(big.Int)

		_, ok := deposit.Amount.SetString(amount, 10)
		if !ok {
			return nil, fmt.Errorf("invalid deposit amount: %s", amount)
		}

		deposits = append(deposits, deposit)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return deposits, nil
}
