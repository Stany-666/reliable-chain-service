package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"reliable-chain-service/internal/ethereum"
	"reliable-chain-service/internal/indexer"
	"reliable-chain-service/internal/scanner"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {

	contractAddress := common.HexToAddress(
		"0xdAC17F958D2ee523a2206206994597C13D831ec7",
	)

	ctx := context.Background()

	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	rpcURL := os.Getenv("ETH_RPC_URL")

	rpc, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatal(err)
	}

	client := &ethereum.Client{

		RPC: rpc,
	}

	latest, err := client.RPC.BlockNumber(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(latest)

	db, err := pgxpool.New(
		ctx,
		os.Getenv("DATABASE_URL"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	repo := indexer.NewRepository(db)

	err = scanner.RunHistorical(ctx, client, repo, contractAddress, 25488876, 3)
	if err != nil {
		log.Fatal(err)
	}
}
