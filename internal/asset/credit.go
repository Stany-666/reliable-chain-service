package asset

import (
	"context"
	"fmt"
)

// CreditDeposit credits a confirmed deposit; false means it was already credited.
// The repository reads the amount and owner from the locked database row.
func (s *Service) CreditDeposit(ctx context.Context, depositID int64) (bool, error) {
	return s.repo.CreditDeposit(ctx, depositID)
}

func (r *Repository) LoadConfirmedDepositIDs(ctx context.Context, limit int) ([]int64, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT id FROM deposits
		WHERE status = $1
		ORDER BY id
		LIMIT $2
	`, DepositStatusConfirmed, limit)
	if err != nil {
		return nil, fmt.Errorf("load confirmed deposits: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan confirmed deposit: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read confirmed deposits: %w", err)
	}
	return ids, nil
}

// CreditDeposit writes the ledger, balance and credited status in one transaction.
func (r *Repository) CreditDeposit(ctx context.Context, depositID int64) (bool, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin deposit credit: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID, assetID int64
	var amount string
	var status DepositStatus

	// 同一笔充值并发入账时，后来的事务等前一个完成后再检查状态。
	err = tx.QueryRow(ctx, `
		SELECT user_id, asset_id, amount::text, status
		FROM deposits WHERE id = $1
		FOR UPDATE
	`, depositID).Scan(&userID, &assetID, &amount, &status)
	if err != nil {
		return false, fmt.Errorf("load deposit %d for credit: %w", depositID, err)
	}
	if status == DepositStatusCredited {
		return false, nil
	}
	if status != DepositStatusConfirmed {
		return false, fmt.Errorf("deposit %d cannot be credited from status %s", depositID, status)
	}

	// 每个用户有可用、冻结两个负债账户，平台有一个托管资产账户。
	_, err = tx.Exec(ctx, `
		INSERT INTO accounts (user_id, asset_id, account_type)
		VALUES (NULL, $2, 'custody'), ($1, $2, 'available'), ($1, $2, 'frozen')
		ON CONFLICT (user_id, asset_id, account_type) DO NOTHING
	`, userID, assetID)
	if err != nil {
		return false, fmt.Errorf("create accounts for deposit %d: %w", depositID, err)
	}

	// 固定按账户 ID 顺序加锁，不同充值并发时使用相同的锁顺序。
	rows, err := tx.Query(ctx, `
		SELECT id, account_type FROM accounts
		WHERE asset_id = $1 AND (user_id = $2 OR account_type = 'custody')
		ORDER BY id FOR UPDATE
	`, assetID, userID)
	if err != nil {
		return false, fmt.Errorf("lock accounts for deposit %d: %w", depositID, err)
	}
	var custodyID, availableID int64
	var accountIDs []int64
	for rows.Next() {
		var id int64
		var accountType string
		if err := rows.Scan(&id, &accountType); err != nil {
			rows.Close()
			return false, fmt.Errorf("read posting account: %w", err)
		}
		accountIDs = append(accountIDs, id)
		switch accountType {
		case "custody":
			custodyID = id
		case "available":
			availableID = id
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read posting accounts: %w", err)
	}
	if len(accountIDs) != 3 || custodyID == 0 || availableID == 0 {
		return false, fmt.Errorf("deposit %d does not have all posting accounts", depositID)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO balances (account_id)
		SELECT unnest($1::bigint[])
		ON CONFLICT (account_id) DO NOTHING
	`, accountIDs)
	if err != nil {
		return false, fmt.Errorf("initialize account balances: %w", err)
	}

	// 唯一业务来源放在主记录上；一笔充值下面写两条借贷分录。
	var transactionID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO ledger_transactions (asset_id, business_type, business_id, deposit_id)
		VALUES ($1, 'deposit', $2, $2) RETURNING id
	`, assetID, depositID).Scan(&transactionID)
	if err != nil {
		return false, fmt.Errorf("create journal for deposit %d: %w", depositID, err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger (transaction_id, account_id, asset_id, direction, amount)
		VALUES ($1, $2, $4, 'debit', $5), ($1, $3, $4, 'credit', $5)
	`, transactionID, custodyID, availableID, assetID, amount)
	if err != nil {
		return false, fmt.Errorf("post entries for deposit %d: %w", depositID, err)
	}

	// 充值同时增加托管资产和用户可用余额，冻结余额保持原值。
	tag, err := tx.Exec(ctx, `
		UPDATE balances SET amount = amount + $2::numeric, updated_at = NOW()
		WHERE account_id = ANY($1::bigint[])
	`, []int64{custodyID, availableID}, amount)
	if err != nil {
		return false, fmt.Errorf("update balance for deposit %d: %w", depositID, err)
	}
	if tag.RowsAffected() != 2 {
		return false, fmt.Errorf("deposit %d did not update both balances", depositID)
	}

	tag, err = tx.Exec(ctx, `
		UPDATE deposits SET status = $1, updated_at = NOW()
		WHERE id = $2 AND status = $3
	`, DepositStatusCredited, depositID, DepositStatusConfirmed)
	if err != nil {
		return false, fmt.Errorf("mark deposit %d credited: %w", depositID, err)
	}
	if tag.RowsAffected() != 1 {
		return false, fmt.Errorf("deposit %d was not marked credited", depositID)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit credit for deposit %d: %w", depositID, err)
	}
	return true, nil
}
