package asset

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func creditTestRepository(t *testing.T) (*Repository, context.Context) {
	t.Helper()
	return creditTestRepositoryThrough(t, "016_drop_legacy_accounting_tables.sql")
}

func creditTestRepositoryThrough(t *testing.T, lastMigration string) (*Repository, context.Context) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("credit_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close(cleanupCtx)
	})
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatal("cannot locate migrations")
	}
	for _, file := range files {
		if filepath.Base(file) > lastMigration {
			continue
		}
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("migration %s: %v", filepath.Base(file), err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit migration %s: %v", filepath.Base(file), err)
		}
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, email) VALUES (1, 'credit-test@example.test');
		INSERT INTO assets (id, chain_id, symbol, token_address, decimals)
		VALUES (1, 1, 'USDT', '0xtest-token', 6);
	`)
	if err != nil {
		t.Fatal(err)
	}
	return NewRepository(pool), ctx
}

func insertCreditTestDeposit(t *testing.T, r *Repository, ctx context.Context, id int64, status DepositStatus) {
	t.Helper()
	_, err := r.DB.Exec(ctx, `
		INSERT INTO mock_transfer_events (
			id, chain_id, tx_hash, log_index, block_number, block_hash,
			token_address, from_address, to_address, amount
		) VALUES ($1, 1, $2, 0, 100, '0xblock', '0xtest-token', '0xfrom', '0xto', 1000000)
	`, id, fmt.Sprintf("0xtest-tx-%d", id))
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.DB.Exec(ctx, `
		INSERT INTO deposits (id, transfer_event_id, user_id, asset_id, amount, status)
		VALUES ($1, $1, 1, 1, 1000000, $2)
	`, id, status)
	if err != nil {
		t.Fatal(err)
	}
}
