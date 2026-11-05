package asset

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

func ValidateWithdrawalAmount(amount string) error {
	n := new(big.Int)
	_, ok := n.SetString(amount, 10)
	if !ok {
		return fmt.Errorf("%w: amount must be an integer", ErrInvalidAmount)
	}
	if n.Sign() <= 0 {
		return fmt.Errorf("%w: amount must be greater than 0", ErrInvalidAmount)
	}
	return nil
}

func ValidateWithdrawalAddress(address string) error {
	if !common.IsHexAddress(address) {
		return fmt.Errorf("%w: not a valid ethereum address", ErrInvalidAddress)
	}
	return nil
}

func (r *Repository) CreateWithdrawal(ctx context.Context, userID, assetID int64, amount string, toAddress string) (int64, error) {

	// 创建提现记录
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin withdrawal: %w", err)
	}
	defer tx.Rollback(ctx)

	//这里写逻辑,检查user_id的资产账户是否存在,如果不存在err,如果存在则检查余额是否足够
	availableID, frozenID, err := lockUserAccounts(ctx, tx, userID, assetID)
	if err != nil {
		return 0, err
	}

	//检查余额
	var enough bool

	err = tx.QueryRow(ctx, `
	SELECT amount >= $2::numeric
	FROM balances
	WHERE account_id = $1
`, availableID, amount).Scan(&enough)

	if err != nil {
		return 0, fmt.Errorf("check available balance: %w", err)
	}
	if !enough {
		return 0, ErrInsufficientBalance
	}

	//frozen账户增加,available账户减少
	//available 减
	tag, err := tx.Exec(ctx, `
	UPDATE balances
	SET amount = amount - $2::numeric, updated_at = NOW()
	WHERE account_id = $1
`, availableID, amount)
	if err != nil {
		return 0, fmt.Errorf("decrease available balance: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return 0, fmt.Errorf("available balance of account %d was not updated", availableID)
	}

	//frozen账户增加
	tag, err = tx.Exec(ctx, `
	UPDATE balances
	SET amount = amount + $2::numeric, updated_at = NOW()
	WHERE account_id = $1
`, frozenID, amount)
	if err != nil {
		return 0, fmt.Errorf("increase frozen balance: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return 0, fmt.Errorf("frozen balance of account %d was not updated", frozenID)
	}

	//创建提现记录
	var withdrawalID int64

	err = tx.QueryRow(ctx, `
	INSERT INTO withdrawals (user_id, asset_id, amount, to_address)
	VALUES ($1, $2, $3::numeric, $4)
	RETURNING id
`, userID, assetID, amount, toAddress).Scan(&withdrawalID)
	if err != nil {
		return 0, fmt.Errorf("create withdrawal: %w", err)
	}

	//创建账务记录
	var transactionID int64

	err = tx.QueryRow(ctx, `
	INSERT INTO ledger_transactions (asset_id, business_type, business_id)
	VALUES ($1, 'freeze', $2)
	RETURNING id
`, assetID, withdrawalID).Scan(&transactionID)
	if err != nil {
		return 0, fmt.Errorf("create journal for withdrawal %d: %w", withdrawalID, err)
	}

	//
	_, err = tx.Exec(ctx, `
	INSERT INTO ledger (transaction_id, account_id, asset_id, direction, amount)
	VALUES ($1, $2, $4, 'debit', $5::numeric), ($1, $3, $4, 'credit', $5::numeric)
`, transactionID, availableID, frozenID, assetID, amount)

	if err != nil {
		return 0, fmt.Errorf("post entries for withdrawal %d: %w", withdrawalID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit withdrawal %d: %w", withdrawalID, err)
	}

	return withdrawalID, nil

}

func (s *Service) CreateWithdrawal(ctx context.Context, userID, assetID int64, amount string, toAddress string) (int64, error) {
	// 第 1 步：校验金额，不合法就返回错误
	if err := ValidateWithdrawalAmount(amount); err != nil {
		return 0, err
	}
	// 第 2 步：校验地址，不合法就返回错误
	if err := ValidateWithdrawalAddress(toAddress); err != nil {
		return 0, err
	}
	// 第 3 步：调用 repository 创建提现，把结果直接返回
	withdrawalID, err := s.repo.CreateWithdrawal(ctx, userID, assetID, amount, toAddress)
	if err != nil {
		return 0, err
	}
	return withdrawalID, nil
}

func (r *Repository) CancelWithdrawal(
	ctx context.Context,
	userID, withdrawalID int64) error {
	// 1. 开启事务，出错就返回
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cancelwithdrawal: %w", err)
	}

	// 2. defer 回滚
	defer tx.Rollback(ctx)

	// 3. 查询并锁住当前用户的这笔提现
	var assetID int64
	var amount string
	var status WithdrawalStatus

	err = tx.QueryRow(ctx, `
	SELECT asset_id, amount::text, status
	FROM withdrawals
	WHERE user_id = $1 AND id = $2
	FOR UPDATE
	`, userID, withdrawalID).Scan(&assetID, &amount, &status)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrWithdrawalNotFound
	}

	if err != nil {
		return fmt.Errorf("lock withdrawal: %w", err)
	}

	// 4. 检查状态是否为 pending
	if status != WithdrawalStatusPending {
		return fmt.Errorf("%w: withdrawal %d", ErrInvalidStatus, withdrawalID)
	}

	// 5. 查询并锁住当前用户的这笔提现的可用账户和冻结账户
	availableID, frozenID, err := lockUserAccounts(ctx, tx, userID, assetID)
	if err != nil {
		return err
	}

	// 6. 创建账务记录
	var transactionID int64
	err = tx.QueryRow(ctx, `
	INSERT INTO ledger_transactions (asset_id, business_type, business_id)
	VALUES ($1, 'unfreeze', $2)
	RETURNING id
	`, assetID, withdrawalID).Scan(&transactionID)
	if err != nil {
		return fmt.Errorf("create journal for cancelwithdrawal %d: %w", withdrawalID, err)
	}

	_, err = tx.Exec(ctx, `
	INSERT INTO ledger (transaction_id, account_id, asset_id, direction, amount)
	VALUES ($1, $3, $4, 'debit', $5::numeric), ($1, $2, $4, 'credit', $5::numeric)
	`, transactionID, availableID, frozenID, assetID, amount)
	if err != nil {
		return fmt.Errorf("post entries for cancelwithdrawal %d: %w", withdrawalID, err)
	}

	// 7. 更新账户余额
	// available 加
	tag, err := tx.Exec(ctx, `
		UPDATE balances
		SET amount = amount + $2::numeric, updated_at = NOW()
		WHERE account_id = $1
	`, availableID, amount)
	if err != nil {
		return fmt.Errorf("increase available balance: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("available balance of account %d was not updated", availableID)
	}

	// frozen 减
	tag, err = tx.Exec(ctx, `
		UPDATE balances
		SET amount = amount - $2::numeric, updated_at = NOW()
		WHERE account_id = $1
	`, frozenID, amount)
	if err != nil {
		return fmt.Errorf("decrease frozen balance: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("frozen balance of account %d was not updated", frozenID)
	}

	tag, err = tx.Exec(ctx, `
	UPDATE withdrawals SET status = $2, updated_at = NOW()
	WHERE id = $1 AND status = $3
	`, withdrawalID, WithdrawalStatusCancelled, WithdrawalStatusPending)
	if err != nil {
		return fmt.Errorf("mark withdrawal %d cancelled: %w", withdrawalID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("withdrawal %d was not marked cancelled", withdrawalID)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit cancelwithdrawal %d: %w", withdrawalID, err)
	}
	return nil
}

func (s *Service) CancelWithdrawal(
	ctx context.Context,
	userID, withdrawalID int64,
) error {
	// 调用 s.repo.CancelWithdrawal，传入这三个参数，并返回它的错误
	return s.repo.CancelWithdrawal(ctx, userID, withdrawalID)
}

func lockUserAccounts(
	ctx context.Context,
	tx pgx.Tx,
	userID, assetID int64,
) (int64, int64, error) {
	// 1. 使用 tx 查询并锁住账户
	// 2. 遍历结果，找到 availableID 和 frozenID
	// 3. 检查读取错误，以及账户是否齐全
	// 4. 返回 availableID、frozenID、错误

	rows, err := tx.Query(ctx, `
		SELECT id, account_type FROM accounts
		WHERE user_id = $1 AND asset_id = $2
		ORDER BY id FOR UPDATE
	`, userID, assetID)
	if err != nil {
		return 0, 0, fmt.Errorf("lock user accounts: %w", err)
	}

	var availableID, frozenID int64
	for rows.Next() {
		var id int64
		var accountType string
		if err := rows.Scan(&id, &accountType); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("read account: %w", err)
		}
		if accountType == "available" {
			availableID = id
		} else if accountType == "frozen" {
			frozenID = id
		}

	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("read user accounts: %w", err)
	}
	if availableID == 0 || frozenID == 0 {
		return 0, 0, fmt.Errorf("%w: user %d has no accounts for asset %d",
			ErrAccountNotFound, userID, assetID)
	}
	return availableID, frozenID, nil
}

// FailWithdrawal 在数据库事务中退回提现本金，并将状态从 processing 改为 failed。
// 调用前提：系统调用方已确认该提现失败，且对应链上交易不会再成功转出本金。
// 本函数不查询链上结果；processing 只表示允许处理，不是链上失败的证据。
// 广播超时、暂时查不到交易或交易尚未打包，都不能作为退款依据。
// 调用方还须保证该提现不会被其他后台任务继续广播或重试转账。
// 当前尚未实现签名、广播和链上结果核对；测试通过设置 processing 模拟调用前提，
// 仅验证退款记账与状态保护，不代表已验证链上失败判定。手续费不在本函数处理范围内。
func (r *Repository) FailWithdrawal(
	ctx context.Context,
	withdrawalID int64,
) error {
	// 1. 开启事务，defer 回滚
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin FailWithdrawal: %w", err)
	}

	defer tx.Rollback(ctx)

	// 2. 按 withdrawalID 查询并锁住提现单
	//    读取 user_id、asset_id、amount、status
	var status WithdrawalStatus
	var userID int64
	var assetID int64
	var amount string

	err = tx.QueryRow(ctx, `
		SELECT user_id, asset_id, amount::text, status
		FROM withdrawals
		WHERE id = $1
		FOR UPDATE
`, withdrawalID).Scan(&userID, &assetID, &amount, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrWithdrawalNotFound
	}
	if err != nil {
		return fmt.Errorf("lock withdrawal for failure: %w", err)
	}

	// 3. 检查状态是否允许失败退款
	if status != WithdrawalStatusProcessing {
		return fmt.Errorf("%w: withdrawal %d", ErrInvalidStatus, withdrawalID)
	}

	// 4. 调用 lockUserAccounts，取得两个账户 ID
	var availableID, frozenID int64
	availableID, frozenID, err = lockUserAccounts(ctx, tx, userID, assetID)
	if err != nil {
		return err
	}

	// 5. 写 unfreeze 账本及分录
	// 6. available 加、frozen 减，检查更新行数
	var transactionID int64
	err = tx.QueryRow(ctx, `
	INSERT INTO ledger_transactions (asset_id, business_type, business_id)
	VALUES ($1, 'unfreeze', $2)
	RETURNING id
`, assetID, withdrawalID).Scan(&transactionID)
	if err != nil {
		return fmt.Errorf("create journal for failwithdrawal %d: %w", withdrawalID, err)
	}

	_, err = tx.Exec(ctx, `
	INSERT INTO ledger (transaction_id, account_id, asset_id, direction, amount)
	VALUES ($1, $3, $4, 'debit', $5::numeric), ($1, $2, $4, 'credit', $5::numeric)
`, transactionID, availableID, frozenID, assetID, amount)
	if err != nil {
		return fmt.Errorf("post entries for failwithdrawal %d: %w", withdrawalID, err)
	}

	// available 加
	tag, err := tx.Exec(ctx, `
		UPDATE balances
		SET amount = amount + $2::numeric, updated_at = NOW()
		WHERE account_id = $1
`, availableID, amount)
	if err != nil {
		return fmt.Errorf("increase available balance: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("available balance of account %d was not updated", availableID)
	}

	// frozen 减
	tag, err = tx.Exec(ctx, `
		UPDATE balances
		SET amount = amount - $2::numeric, updated_at = NOW()
		WHERE account_id = $1
`, frozenID, amount)
	if err != nil {
		return fmt.Errorf("decrease frozen balance: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("frozen balance of account %d was not updated", frozenID)
	}

	tag, err = tx.Exec(ctx, `
	UPDATE withdrawals SET status = $2, updated_at = NOW()
	WHERE id = $1 AND status = $3
`, withdrawalID, WithdrawalStatusFailed, WithdrawalStatusProcessing)
	if err != nil {
		return fmt.Errorf("mark withdrawal %d Failed: %w", withdrawalID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("withdrawal %d was not marked Failed", withdrawalID)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Failedwithdrawal %d: %w", withdrawalID, err)
	}
	return nil

	// 7. 状态改成 failed，检查更新行数

	// 8. 提交事务

}

// ConfirmWithdrawal 的调用方须先确认链上提现成功，并满足系统的确认要求。
// 本函数只处理数据库扣账，不负责签名、广播或查询链上结果。
func (r *Repository) ConfirmWithdrawal(
	ctx context.Context,
	withdrawalID int64,
) error {
	// 1. 开启事务，defer 回滚
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Confirm Withdrawal: %w", err)
	}

	defer tx.Rollback(ctx)
	// 2. 查询并锁住提现单，读取 user_id、asset_id、amount、status
	var status WithdrawalStatus
	var userID int64
	var assetID int64
	var amount string

	err = tx.QueryRow(ctx, `
		SELECT user_id, asset_id, amount::text, status
		FROM withdrawals
		WHERE id = $1
		FOR UPDATE
	`, withdrawalID).Scan(&userID, &assetID, &amount, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrWithdrawalNotFound
	}
	if err != nil {
		return fmt.Errorf("lock withdrawal for confirmation: %w", err)
	}

	// 3. 检查状态必须是 processing
	if status != WithdrawalStatusProcessing {
		return fmt.Errorf("%w: withdrawal %d", ErrInvalidStatus, withdrawalID)
	}

	// 4. 按账户 ID 顺序锁住相关账户，取得 frozenID 和 custodyID

	rows, err := tx.Query(ctx, `
    SELECT id, account_type
    FROM accounts
    WHERE asset_id = $1
      AND (
          (user_id IS NULL AND account_type = 'custody')
          OR (user_id = $2 AND account_type = 'frozen')
      )
    ORDER BY id FOR UPDATE
`, assetID, userID)

	if err != nil {
		return fmt.Errorf("lock withdrawal accounts: %w", err)
	}

	var custodyID, frozenID int64
	for rows.Next() {
		var id int64
		var accountType string
		if err := rows.Scan(&id, &accountType); err != nil {
			rows.Close()
			return fmt.Errorf("read account: %w", err)
		}
		if accountType == "frozen" {
			frozenID = id
		} else if accountType == "custody" {
			custodyID = id
		}

	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read user accounts: %w", err)
	}
	if custodyID == 0 || frozenID == 0 {
		return fmt.Errorf("%w: user %d has no accounts for asset %d",
			ErrAccountNotFound, userID, assetID)
	}

	// 5. 写 withdrawal 主记录及两条分录

	// 6. frozen、custody 各减 amount，检查更新行数

	// 7. 状态改成 confirmed，检查更新行数
	var transactionID int64
	err = tx.QueryRow(ctx, `
	INSERT INTO ledger_transactions (asset_id, business_type, business_id)
	VALUES ($1, 'withdrawal', $2)
	RETURNING id
`, assetID, withdrawalID).Scan(&transactionID)
	if err != nil {
		return fmt.Errorf("create journal for ConfirmWithdrawal %d: %w", withdrawalID, err)
	}

	_, err = tx.Exec(ctx, `
	INSERT INTO ledger (transaction_id, account_id, asset_id, direction, amount)
	VALUES ($1, $3, $4, 'debit', $5::numeric), ($1, $2, $4, 'credit', $5::numeric)
`, transactionID, custodyID, frozenID, assetID, amount)
	if err != nil {
		return fmt.Errorf("post entries for ConfirmWithdrawal %d: %w", withdrawalID, err)
	}

	// custody 减
	tag, err := tx.Exec(ctx, `
		UPDATE balances
		SET amount = amount - $2::numeric, updated_at = NOW()
		WHERE account_id = $1
`, custodyID, amount)
	if err != nil {
		return fmt.Errorf("decrease custody balance: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("custody balance of account %d was not updated", custodyID)
	}

	// frozen 减
	tag, err = tx.Exec(ctx, `
		UPDATE balances
		SET amount = amount - $2::numeric, updated_at = NOW()
		WHERE account_id = $1
`, frozenID, amount)
	if err != nil {
		return fmt.Errorf("decrease frozen balance: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("frozen balance of account %d was not updated", frozenID)
	}

	tag, err = tx.Exec(ctx, `
	UPDATE withdrawals SET status = $2, updated_at = NOW()
	WHERE id = $1 AND status = $3
`, withdrawalID, WithdrawalStatusConfirmed, WithdrawalStatusProcessing)
	if err != nil {
		return fmt.Errorf("mark withdrawal %d Confirmed: %w", withdrawalID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("withdrawal %d was not marked Confirm", withdrawalID)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Confirmedwithdrawal %d: %w", withdrawalID, err)
	}
	return nil

}
