package asset

import (
	"errors"
	"sync"
	"testing"
)

func TestCreateWithdrawalSuccess(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 准备：给用户 1 充值，让他账上有钱
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}

	// 创建提现
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}
	if withdrawalID <= 0 {
		t.Fatalf("withdrawal ID = %d, want a positive ID", withdrawalID)
	}

	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Available.String() != "700000" {
		t.Fatalf("balance.Available = %s, want 700000", balance.Available.String())
	}
	if balance.Frozen.String() != "300000" {
		t.Fatalf("balance.Frozen = %s, want 300000", balance.Frozen.String())
	}
}

func TestCancelWithdrawalSuccess(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 准备：充值 1000000，再提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 执行：取消这笔提现
	if err := NewService(r, nil).CancelWithdrawal(ctx, 1, withdrawalID); err != nil {
		t.Fatal(err)
	}

	// 检查：你来写
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "1000000" {
		t.Fatalf("balance.Available = %s, want 1000000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}
	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE user_id = $1 AND id = $2
	`, 1, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusCancelled {
		t.Fatalf("state = %s, want cancelled", state)
	}
}

func TestCancelWithdrawalTwice(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 准备：充值 1000000，再提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 执行：取消这笔提现
	if err := r.CancelWithdrawal(ctx, 1, withdrawalID); err != nil {
		t.Fatal(err)
	}

	err = r.CancelWithdrawal(ctx, 1, withdrawalID)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	// 检查：你来写
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "1000000" {
		t.Fatalf("balance.Available = %s, want 1000000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}

}

func TestCancelWithdrawalOtherUser(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 充值 1000000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// 提现 300000
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 执行：取消这笔提现
	err = r.CancelWithdrawal(ctx, 2, withdrawalID)
	if !errors.Is(err, ErrWithdrawalNotFound) {
		t.Fatalf("expected ErrWithdrawalNotFound, got %v", err)
	}

	// 检查：你来写
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "700000" {
		t.Fatalf("balance.Available = %s, want 700000", balance.Available.String())
	}
	if balance.Frozen.String() != "300000" {
		t.Fatalf("balance.Frozen = %s, want 300000", balance.Frozen.String())
	}

}

func TestCreateWithdrawalInsufficientBalance(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 准备：给用户 1 充值 1000000
	// 充值 1000000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// 2. 执行：申请提现 2000000
	toAddress := "0x1111111111111111111111111111111111111111"
	_, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "2000000", toAddress)

	// 3. 检查：必须返回 ErrInsufficientBalance
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}

	// 4. 检查：available 仍为 1000000，frozen 仍为 0
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "1000000" {
		t.Fatalf("balance.Available = %s, want 1000000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}
}

func TestCreateWithdrawalAccountNotFound(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 不充值，让用户 1 没有该资产的账户
	// 2. 通过 service 申请提现，使用合法金额和地址
	toAddress := "0x1111111111111111111111111111111111111111"
	_, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "1", toAddress)

	// 3. 用 errors.Is 检查必须返回 ErrAccountNotFound
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestFailWithdrawalSuccess(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 充值 1000000，再申请提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 把提现状态从 pending 改成 processing，模拟后台接手
	tag, err := r.DB.Exec(ctx, `
		UPDATE withdrawals
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, withdrawalID, WithdrawalStatusProcessing, WithdrawalStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated %d withdrawals, want 1", tag.RowsAffected())
	}

	// 3. 调用 r.FailWithdrawal
	err = r.FailWithdrawal(ctx, withdrawalID)
	if err != nil {
		t.Fatal(err)
	}

	// 4. 检查 available == 1000000、frozen == 0
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "1000000" {
		t.Fatalf("balance.Available = %s, want 1000000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}

	// 5. 检查提现状态 == WithdrawalStatusFailed
	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusFailed {
		t.Fatalf("state = %s, want failed", state)
	}

}

func TestFailWithdrawalTwice(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 充值 1000000，再申请提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 把提现状态从 pending 改成 processing，模拟后台接手
	tag, err := r.DB.Exec(ctx, `
		UPDATE withdrawals
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, withdrawalID, WithdrawalStatusProcessing, WithdrawalStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated %d withdrawals, want 1", tag.RowsAffected())
	}

	// 3. 调用 r.FailWithdrawal
	err = r.FailWithdrawal(ctx, withdrawalID)
	if err != nil {
		t.Fatal(err)
	}
	//4. 再次调用
	err = r.FailWithdrawal(ctx, withdrawalID)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	// 5. 检查余额状态
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "1000000" {
		t.Fatalf("balance.Available = %s, want 1000000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}

	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusFailed {
		t.Fatalf("state = %s, want failed", state)
	}

}

func TestFailWithdrawalPending(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 充值 1000000，再申请提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 调用,期待ErrInvalidStatus
	err = r.FailWithdrawal(ctx, withdrawalID)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	// 3. 检查余额状态
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "700000" {
		t.Fatalf("balance.Available = %s, want 700000", balance.Available.String())
	}
	if balance.Frozen.String() != "300000" {
		t.Fatalf("balance.Frozen = %s, want 300000", balance.Frozen.String())
	}

	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusPending {
		t.Fatalf("state = %s, want failed", state)
	}

}

func TestFailWithdrawalCancelled(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 准备：充值 1000000，再提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 执行：取消这笔提现
	if err := NewService(r, nil).CancelWithdrawal(ctx, 1, withdrawalID); err != nil {
		t.Fatal(err)
	}

	// 调用期待ErrInvalidStatus
	err = r.FailWithdrawal(ctx, withdrawalID)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	// 检查余额状态
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "1000000" {
		t.Fatalf("balance.Available = %s, want 1000000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}

	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusCancelled {
		t.Fatalf("state = %s, want cancelled", state)
	}

}

func TestFailWithdrawalNotFound(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	err := r.FailWithdrawal(ctx, 1)
	if !errors.Is(err, ErrWithdrawalNotFound) {
		t.Fatalf("expected ErrWithdrawalNotFound, got %v", err)
	}

}

func TestFailWithdrawalLedger(t *testing.T) {
	// 1. 创建测试环境
	// 2. 充值 1000000，申请提现 300000
	// 3. 把提现状态改成 processing
	// 4. 调用 FailWithdrawal，检查成功
	// 5. 查询并检查退款账本

	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 充值 1000000，再申请提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 把提现状态从 pending 改成 processing，模拟后台接手
	tag, err := r.DB.Exec(ctx, `
		UPDATE withdrawals
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, withdrawalID, WithdrawalStatusProcessing, WithdrawalStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated %d withdrawals, want 1", tag.RowsAffected())
	}

	// 3. 调用 r.FailWithdrawal
	err = r.FailWithdrawal(ctx, withdrawalID)
	if err != nil {
		t.Fatal(err)
	}

	// 4. 检查 available == 1000000、frozen == 0
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "1000000" {
		t.Fatalf("balance.Available = %s, want 1000000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}

	// 5. 检查提现状态 == WithdrawalStatusFailed
	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusFailed {
		t.Fatalf("state = %s, want failed", state)
	}

	var transactionID int64

	err = r.DB.QueryRow(ctx, `
    SELECT id
    FROM ledger_transactions
    WHERE business_id = $1 AND business_type = 'unfreeze'
	`, withdrawalID).Scan(&transactionID)
	if err != nil {
		t.Fatal(err)
	}

	type LedgerEntry struct {
		AccountID int64
		Direction string
		Amount    string
	}

	rows, err := r.DB.Query(ctx, `
		SELECT direction, amount
		FROM ledger
		WHERE transaction_id = $1
	`, transactionID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	debitCount, creditCount := 0, 0

	for rows.Next() {
		var entry LedgerEntry
		if err := rows.Scan(&entry.Direction, &entry.Amount); err != nil {
			t.Fatal(err)
		}

		if entry.Amount != "300000" {
			t.Fatalf("amount = %s, want 300000", entry.Amount)
		}

		switch entry.Direction {
		case "debit":
			debitCount++
		case "credit":
			creditCount++
		default:
			t.Fatalf("unexpected direction: %s", entry.Direction)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if debitCount != 1 || creditCount != 1 {
		t.Fatalf("debit count = %d, credit count = %d, want 1 each",
			debitCount, creditCount)
	}
}

func TestConfirmWithdrawalSuccess(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 充值 1000000，再申请提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 把提现状态从 pending 改成 processing，模拟后台接手
	tag, err := r.DB.Exec(ctx, `
		UPDATE withdrawals
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, withdrawalID, WithdrawalStatusProcessing, WithdrawalStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated %d withdrawals, want 1", tag.RowsAffected())
	}

	err = r.ConfirmWithdrawal(ctx, withdrawalID)
	if err != nil {
		t.Fatal(err)
	}

	// 4. 检查 available == 700000、frozen == 0
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "700000" {
		t.Fatalf("balance.Available = %s, want 700000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}

	var custodybalance string
	custodybalance, err = r.GetCustodyBalance(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if custodybalance != "700000" {
		t.Fatalf("custodybalance = %s, want 700000", custodybalance)
	}

	// 5. 检查提现状态 == WithdrawalStatusConfirmed
	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusConfirmed {
		t.Fatalf("state = %s, want confirmed", state)

	}
}

func TestConfirmWithdrawalTwice(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 充值 1000000，再申请提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 把提现状态从 pending 改成 processing，模拟后台接手
	tag, err := r.DB.Exec(ctx, `
		UPDATE withdrawals
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, withdrawalID, WithdrawalStatusProcessing, WithdrawalStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated %d withdrawals, want 1", tag.RowsAffected())
	}

	err = r.ConfirmWithdrawal(ctx, withdrawalID)
	if err != nil {
		t.Fatal(err)
	}

	// 3. 再次确认，检查是否报错
	err = r.ConfirmWithdrawal(ctx, withdrawalID)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	// 4. 检查 available == 700000、frozen == 0
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "700000" {
		t.Fatalf("balance.Available = %s, want 700000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}

	var custodybalance string
	custodybalance, err = r.GetCustodyBalance(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if custodybalance != "700000" {
		t.Fatalf("custodybalance = %s, want 700000", custodybalance)
	}

	// 5. 检查提现状态 == WithdrawalStatusConfirmed
	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusConfirmed {
		t.Fatalf("state = %s, want confirmed", state)

	}
}

func TestConfirmWithdrawalPending(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 充值 1000000，再申请提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 把提现状态从 pending 改成 pending
	tag, err := r.DB.Exec(ctx, `
		UPDATE withdrawals
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, withdrawalID, WithdrawalStatusPending, WithdrawalStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated %d withdrawals, want 1", tag.RowsAffected())
	}

	// 3. 检查是否报错
	err = r.ConfirmWithdrawal(ctx, withdrawalID)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	// 4. 检查 available == 700000、frozen == 300000
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "700000" {
		t.Fatalf("balance.Available = %s, want 700000", balance.Available.String())
	}
	if balance.Frozen.String() != "300000" {
		t.Fatalf("balance.Frozen = %s, want 300000", balance.Frozen.String())
	}

	var custodybalance string
	custodybalance, err = r.GetCustodyBalance(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if custodybalance != "1000000" {
		t.Fatalf("custodybalance = %s, want 1000000", custodybalance)
	}

	// 5. 检查提现状态 == WithdrawalStatusPending
	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusPending {
		t.Fatalf("state = %s, want Pending", state)

	}
}

func TestConfirmWithdrawalCancelled(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")
	// 1. 充值 1000000，再申请提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 把提现状态从 pending 改成 pending
	tag, err := r.DB.Exec(ctx, `
		UPDATE withdrawals
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, withdrawalID, WithdrawalStatusPending, WithdrawalStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated %d withdrawals, want 1", tag.RowsAffected())
	}

	//3.CancelWithdrawal
	err = r.CancelWithdrawal(ctx, 1, withdrawalID)
	if err != nil {
		t.Fatal(err)
	}

	// 4. 检查是否报错
	err = r.ConfirmWithdrawal(ctx, withdrawalID)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	// 5. 检查 available == 1000000、frozen == 0
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "1000000" {
		t.Fatalf("balance.Available = %s, want 1000000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}

	var custodybalance string
	custodybalance, err = r.GetCustodyBalance(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if custodybalance != "1000000" {
		t.Fatalf("custodybalance = %s, want 1000000", custodybalance)
	}

	// 6. 检查提现状态 == WithdrawalStatusCancelled
	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusCancelled {
		t.Fatalf("state = %s, want cancelled", state)

	}
}

func TestConfirmWithdrawalFailed(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 充值 1000000，再申请提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 把提现状态从 pending 改成 processing，模拟后台接手
	tag, err := r.DB.Exec(ctx, `
		UPDATE withdrawals
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, withdrawalID, WithdrawalStatusProcessing, WithdrawalStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated %d withdrawals, want 1", tag.RowsAffected())
	}

	// 3. 把提现状态从 processing 改成 failed
	err = r.FailWithdrawal(ctx, withdrawalID)
	if err != nil {
		t.Fatal(err)
	}

	err = r.ConfirmWithdrawal(ctx, withdrawalID)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	// 5. 检查 available == 1000000、frozen == 0
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "1000000" {
		t.Fatalf("balance.Available = %s, want 1000000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}

	var custodybalance string
	custodybalance, err = r.GetCustodyBalance(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if custodybalance != "1000000" {
		t.Fatalf("custodybalance = %s, want 1000000", custodybalance)
	}

	// 6. 检查提现状态 == WithdrawalStatusFailed
	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusFailed {
		t.Fatalf("state = %s, want failed", state)

	}
}

func TestConfirmWithdrawalNotFound(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	err := r.ConfirmWithdrawal(ctx, 1)
	if !errors.Is(err, ErrWithdrawalNotFound) {
		t.Fatalf("expected ErrWithdrawalNotFound, got %v", err)
	}
}

func TestConfirmWithdrawalLedger(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 充值 1000000，再申请提现 300000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"
	withdrawalID, err := NewService(r, nil).CreateWithdrawal(ctx, 1, 1, "300000", toAddress)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 把提现状态从 pending 改成 processing，模拟后台接手
	tag, err := r.DB.Exec(ctx, `
		UPDATE withdrawals
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, withdrawalID, WithdrawalStatusProcessing, WithdrawalStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated %d withdrawals, want 1", tag.RowsAffected())
	}

	// 3. 调用 r.ConfirmWithdrawal
	err = r.ConfirmWithdrawal(ctx, withdrawalID)
	if err != nil {
		t.Fatal(err)
	}

	// 4. 检查 available == 700000、frozen == 0
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "700000" {
		t.Fatalf("balance.Available = %s, want 700000", balance.Available.String())
	}
	if balance.Frozen.String() != "0" {
		t.Fatalf("balance.Frozen = %s, want 0", balance.Frozen.String())
	}

	var custodybalance string
	custodybalance, err = r.GetCustodyBalance(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if custodybalance != "700000" {
		t.Fatalf("custodybalance = %s, want 700000", custodybalance)
	}

	// 5. 检查提现状态 == WithdrawalStatusConfirmed
	var state WithdrawalStatus
	err = r.DB.QueryRow(ctx, `
	SELECT status
	FROM withdrawals
	WHERE id = $1
	`, withdrawalID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state != WithdrawalStatusConfirmed {
		t.Fatalf("state = %s, want confirmed", state)

	}

	var transactionID int64

	err = r.DB.QueryRow(ctx, `
    SELECT id
    FROM ledger_transactions
    WHERE business_id = $1 AND business_type = 'withdrawal'
	`, withdrawalID).Scan(&transactionID)
	if err != nil {
		t.Fatal(err)
	}

	type LedgerEntry struct {
		Direction string
		Amount    string
	}

	rows, err := r.DB.Query(ctx, `
		SELECT direction, amount
		FROM ledger
		WHERE transaction_id = $1
	`, transactionID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	debitCount, creditCount := 0, 0

	for rows.Next() {
		var entry LedgerEntry
		if err := rows.Scan(&entry.Direction, &entry.Amount); err != nil {
			t.Fatal(err)
		}

		if entry.Amount != "300000" {
			t.Fatalf("amount = %s, want 300000", entry.Amount)
		}

		switch entry.Direction {
		case "debit":
			debitCount++
		case "credit":
			creditCount++
		default:
			t.Fatalf("unexpected direction: %s", entry.Direction)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if debitCount != 1 || creditCount != 1 {
		t.Fatalf("debit count = %d, credit count = %d, want 1 each",
			debitCount, creditCount)
	}
}

func TestCreateWithdrawalConcurrent(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "018_create_withdrawals.sql")

	// 1. 充值 1000000
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	toAddress := "0x1111111111111111111111111111111111111111"

	// 2. 准备两个并发任务，每个申请提现 700000
	var err1, err2 error
	wg := sync.WaitGroup{}

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err1 = r.CreateWithdrawal(ctx, 1, 1, "700000", toAddress)
	}()
	go func() {
		defer wg.Done()
		_, err2 = r.CreateWithdrawal(ctx, 1, 1, "700000", toAddress)
	}()

	wg.Wait()

	// 3. 等待两个任务结束，收集结果
	// 4. 检查一次成功、一次 ErrInsufficientBalance
	if err1 == nil && errors.Is(err2, ErrInsufficientBalance) {
		// 任务 1 成功，任务 2 余额不足
	} else if err2 == nil && errors.Is(err1, ErrInsufficientBalance) {
		// 任务 2 成功，任务 1 余额不足
	} else {
		t.Fatalf("want one success and one ErrInsufficientBalance, got err1=%v, err2=%v",
			err1, err2)
	}

	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Available.String() != "300000" {
		t.Fatalf("balance.Available = %s, want 300000", balance.Available.String())
	}
	if balance.Frozen.String() != "700000" {
		t.Fatalf("balance.Frozen = %s, want 700000", balance.Frozen.String())
	}

	var custodybalance string
	custodybalance, err = r.GetCustodyBalance(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if custodybalance != "1000000" {
		t.Fatalf("custodybalance = %s, want 1000000", custodybalance)
	}

	// 6. 检查只有一笔提现单
	var withdrawalCount int
	err = r.DB.QueryRow(ctx, `
    SELECT COUNT(*)
    FROM withdrawals
    WHERE user_id = $1 AND asset_id = $2
`, 1, 1).Scan(&withdrawalCount)
	if err != nil {
		t.Fatal(err)
	}
	if withdrawalCount != 1 {
		t.Fatalf("withdrawal count = %d, want 1", withdrawalCount)
	}

}
