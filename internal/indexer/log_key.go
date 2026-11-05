package indexer

import (
	"fmt"

	"github.com/ethereum/go-ethereum/core/types"
)

func LogKey(log types.Log) string {
	return fmt.Sprintf("%s:%d", log.TxHash.Hex(), log.Index)
}
