package model

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type TransferEvent struct {
	TxHash       string
	LogIndex     uint64
	BlockNumber  uint64
	TokenAddress common.Address

	From      common.Address
	To        common.Address
	Amount    *big.Int
	BlockHash string
}
