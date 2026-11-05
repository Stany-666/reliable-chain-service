package scanner

import (
	"errors"
	"math/big"
	"reliable-chain-service/internal/model"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func DecodeTransfer(log types.Log) (*model.TransferEvent, error) {

	if len(log.Topics) != 3 {
		return nil, errors.New("invalid transfer log topics")
	}

	from := common.BytesToAddress(
		log.Topics[1].Bytes()[12:],
	)

	to := common.BytesToAddress(
		log.Topics[2].Bytes()[12:],
	)

	amount := new(big.Int).SetBytes(log.Data)

	return &model.TransferEvent{
		TxHash:       log.TxHash.Hex(),
		LogIndex:     uint64(log.Index),
		BlockNumber:  log.BlockNumber,
		TokenAddress: log.Address,

		From:      from,
		To:        to,
		Amount:    amount,
		BlockHash: log.BlockHash.Hex(),
	}, nil
}
