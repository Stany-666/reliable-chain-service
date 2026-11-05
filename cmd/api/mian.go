package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"reliable-chain-service/internal/asset"
	"reliable-chain-service/internal/auth"
	"reliable-chain-service/internal/user"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// 1. 先检查配置，避免连接数据库后才发现缺少签名密钥。
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	secret := []byte(os.Getenv("JWT_SECRET"))
	if len(secret) < 32 {
		return errors.New("JWT_SECRET must be at least 32 bytes")
	}
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startupCtx, cancelStartup := context.WithTimeout(ctx, 10*time.Second)
	defer cancelStartup()

	// 2. 用户模块使用 database/sql，资产模块使用 pgxpool，连接同一个数据库。
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open user database: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(startupCtx); err != nil {
		return fmt.Errorf("connect user database: %w", err)
	}

	pool, err := pgxpool.New(startupCtx, databaseURL)
	if err != nil {
		return fmt.Errorf("open asset database: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(startupCtx); err != nil {
		return fmt.Errorf("connect asset database: %w", err)
	}
	cancelStartup()
	log.Println("database connected")

	// 3. 把 repository、service、handler 串起来。
	userRepo := user.NewRepository(db)
	userService := user.NewService(userRepo, secret)
	userHandler := user.NewHandler(userService)
	assetRepo := asset.NewRepository(pool)
	assetService := asset.NewService(assetRepo, nil)
	assetHandler := asset.NewHandler(assetService)

	// 4. 注册、登录公开；提现、取消必须先经过认证中间件。
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/register", userHandler.CreateUser)
	mux.HandleFunc("POST /api/login", userHandler.LoginUser)
	mux.Handle("POST /api/withdrawals",
		auth.RequireAuth(secret, http.HandlerFunc(assetHandler.CreateWithdrawal)),
	)
	mux.Handle("POST /api/withdrawals/{id}/cancel",
		auth.RequireAuth(secret, http.HandlerFunc(assetHandler.CancelWithdrawal)),
	)

	// 5. 启动 HTTP 服务，限制读取请求和写入响应的等待时间。
	server := &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen API: %w", err)
	}
	defer listener.Close()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	log.Printf("API listening on http://%s", listener.Addr())
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve API: %w", err)
	case <-ctx.Done():
		// 6. Ctrl+C 或 SIGTERM：先等待正在处理的请求，再关闭数据库。
		stop() // 恢复默认信号行为，等待期间再次 Ctrl+C 可以直接退出。
		log.Println("shutting down API")
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown API: %w", err)
		}
		return nil
	}
}
