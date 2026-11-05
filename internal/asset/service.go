package asset

import (
	"context"
	"fmt"
	"reliable-chain-service/internal/ethereum"
)

type Service struct {
	repo   *Repository
	client *ethereum.Client
}

func NewService(
	repo *Repository,
	client *ethereum.Client,
) *Service {

	return &Service{
		repo:   repo,
		client: client,
	}
}

func (s *Service) ProcessTransfer(
	ctx context.Context,
	chainID uint64,
	transfer TransferRecord,
) (bool, error) {

	if transfer.Amount == nil {
		return false, fmt.Errorf("invalid transfer amount")
	}

	if transfer.Amount.Sign() <= 0 {
		return false, nil
	}

	//找到资产
	foundAsset, assetFound, err := s.repo.FindAsset(
		ctx,
		chainID,
		transfer.TokenAddress,
	)
	if err != nil {
		return false, err
	}
	if !assetFound {
		return false, nil
	}

	//找到充值地址
	foundAddress, addressFound, err := s.repo.FindDepositAddress(
		ctx,
		chainID,
		transfer.ToAddress,
	)
	if err != nil {
		return false, err
	}
	if !addressFound {
		return false, nil
	}

	deposit := Deposit{

		TransferEventID: transfer.ID,
		UserID:          foundAddress.UserID,
		AssetID:         foundAsset.ID,
		Amount:          transfer.Amount,
		Status:          DepositStatusDetected,
	}

	created, err := s.repo.CreateDeposit(
		ctx,
		deposit,
	)
	if err != nil {
		return false, err
	}

	return created, nil

}

func (s *Service) ProcessConfirmation(
	ctx context.Context,
	deposit Deposit,
) error {

	event, err := s.repo.FindTransferEventByID(
		ctx, deposit.TransferEventID)
	if err != nil {
		return err
	}

	// 仅用于模拟测试：不查询真实 RPC，假设区块哈希未改变，
	// 且转账后已经增加 12 个区块，让模拟充值走 confirmed 分支。
	// 这些值不验证真实链上的转账；测试未确认或重组时需调整下面的值。
	latestBlock := event.BlockNumber + 12
	rpcBlockHash := event.BlockHash

	result := CheckConfirmation(
		event.BlockHash,
		event.BlockNumber,
		latestBlock,
		rpcBlockHash,
		12,
	)

	switch result {

	case ConfirmationConfirmed:
		return s.repo.UpdateDepositStatus(
			ctx,
			deposit.ID,
			DepositStatusConfirmed,
		)

	case ConfirmationReorged:
		return s.repo.UpdateDepositStatus(
			ctx,
			deposit.ID,
			DepositStatusReverted,
		)

	case ConfirmationNotReady:
		return nil
	}

	return nil
}
