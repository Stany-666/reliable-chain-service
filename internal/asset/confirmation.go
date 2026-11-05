package asset

type ConfirmationResult string

const (
	ConfirmationNotReady  ConfirmationResult = "not_ready"
	ConfirmationConfirmed ConfirmationResult = "confirmed"
	ConfirmationReorged   ConfirmationResult = "reorged"
)

func CheckConfirmation(
	blockHash string,
	blockNumber uint64,
	latestBlock uint64,
	rpcBlockHash string,
	requiredConfirmations uint64,
) ConfirmationResult {

	if latestBlock < blockNumber {
		return ConfirmationNotReady
	}

	if rpcBlockHash != blockHash {
		return ConfirmationReorged
	}

	if latestBlock-blockNumber < requiredConfirmations {
		return ConfirmationNotReady
	}

	return ConfirmationConfirmed
}
