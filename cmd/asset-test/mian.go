package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"reliable-chain-service/internal/asset"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/joho/godotenv"
)

func main() {

	ctx := context.Background()

	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := pgxpool.New(
		ctx,
		os.Getenv("DATABASE_URL"),
	)

	if err != nil {
		panic(err)
	}

	defer pool.Close()

	repo := asset.NewRepository(pool)

	// 模拟充值确认直接使用测试值，不初始化真实 Ethereum RPC。
	service := asset.NewService(repo, nil)

	transfers, err := repo.LoadUnprocessedTransfers(
		ctx,
		2000,
	)

	if err != nil {
		panic(err)
	}

	for _, transfer := range transfers {

		created, err := service.ProcessTransfer(
			ctx,
			1,
			transfer,
		)

		if err != nil {
			panic(err)
		}

		if created {
			fmt.Println("deposit created")
		}
	}

	deposits, err := repo.LoadPendingDeposits(ctx, 200)
	if err != nil {
		panic(err)
	}

	for _, deposit := range deposits {

		err = service.ProcessConfirmation(ctx, deposit)

		if err != nil {
			panic(err)
		}

	}

	// 确认后入账：重复运行时，已 credited 的充值不会再次增加余额。
	confirmedIDs, err := repo.LoadConfirmedDepositIDs(ctx, 200)
	if err != nil {
		panic(err)
	}
	for _, depositID := range confirmedIDs {
		credited, err := service.CreditDeposit(ctx, depositID)
		if err != nil {
			panic(err)
		}
		if credited {
			fmt.Println("deposit credited", depositID)
		}
	}

}
