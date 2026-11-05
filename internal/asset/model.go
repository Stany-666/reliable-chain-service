package asset

import "math/big"

type Asset struct {
	ID           int64
	ChainID      uint64
	Symbol       string
	TokenAddress string
	Decimals     int16
}

type DepositAddress struct {
	UserID  int64
	ChainID uint64
	Address string
}

type DepositStatus string

const (
	DepositStatusDetected  DepositStatus = "detected"
	DepositStatusConfirmed DepositStatus = "confirmed"
	DepositStatusCredited  DepositStatus = "credited"
	DepositStatusReverted  DepositStatus = "reverted"
)

type Deposit struct {
	ID              int64
	TransferEventID int64
	UserID          int64
	AssetID         int64
	Amount          *big.Int
	Status          DepositStatus
}

type TransferRecord struct {
	ID int64

	TxHash       string
	LogIndex     uint64
	BlockNumber  uint64
	BlockHash    string
	TokenAddress string
	FromAddress  string
	ToAddress    string
	Amount       *big.Int
}

type WithdrawalStatus string

const (
	WithdrawalStatusPending    WithdrawalStatus = "pending"
	WithdrawalStatusCancelled  WithdrawalStatus = "cancelled"
	WithdrawalStatusProcessing WithdrawalStatus = "processing"
	WithdrawalStatusConfirmed  WithdrawalStatus = "confirmed"
	WithdrawalStatusFailed     WithdrawalStatus = "failed"
)

type CreateWithdrawalRequest struct {
	AssetID   int64  `json:"asset_id"`
	Amount    string `json:"amount"`
	ToAddress string `json:"to_address"`
}
