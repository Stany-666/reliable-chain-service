package asset

import "errors"

var (
	ErrInsufficientBalance = errors.New("insufficient balance") // 余额不足
	ErrInvalidAmount       = errors.New("invalid amount")       // 金额无效
	ErrInvalidAddress      = errors.New("invalid address")      // 地址无效
	ErrInvalidStatus       = errors.New("invalid status")       // 状态无效
	ErrWithdrawalNotFound  = errors.New("withdrawal not found") // 提现不存在
	ErrAccountNotFound     = errors.New("account not found")    // 账户不存在
)
