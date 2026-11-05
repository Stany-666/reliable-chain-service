package asset

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Each test applies the real migrations to its own schema, never public tables.
// Run with TEST_DATABASE_URL pointing to a local PostgreSQL test database.

func assertCreditTotals(t *testing.T, r *Repository, ctx context.Context, entries int, amount string) {
	t.Helper()
	var count, journals int
	var balance, ledgerAmount, custody, frozen string
	err := r.DB.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM ledger),
		       (SELECT count(*) FROM ledger_transactions),
		       (SELECT coalesce(sum(amount) FILTER (WHERE direction = 'credit'), 0)::text FROM ledger),
		       coalesce(sum(b.amount) FILTER (WHERE a.account_type = 'available'), 0)::text,
		       coalesce(sum(b.amount) FILTER (WHERE a.account_type = 'custody'), 0)::text,
		       coalesce(sum(b.amount) FILTER (WHERE a.account_type = 'frozen'), 0)::text
		FROM accounts a JOIN balances b ON b.account_id = a.id
	`).Scan(&count, &journals, &ledgerAmount, &balance, &custody, &frozen)
	if err != nil {
		t.Fatal(err)
	}
	if count != entries*2 || journals != entries || balance != amount || custody != amount || ledgerAmount != amount || frozen != "0" {
		t.Fatalf("entries=%d journals=%d credit=%s available=%s custody=%s frozen=%s; want %d journals and %s",
			count, journals, ledgerAmount, balance, custody, frozen, entries, amount)
	}
}

func TestCreditDepositRetryAfterReconnect(t *testing.T) {
	r, ctx := creditTestRepository(t)
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	ids, err := r.LoadConfirmedDepositIDs(ctx, 10)
	if err != nil || len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("confirmed deposits=%v err=%v", ids, err)
	}
	if credited, err := NewService(r, nil).CreditDeposit(ctx, 1); err != nil || !credited {
		t.Fatalf("first credit=%v err=%v", credited, err)
	}
	// A fresh pool checks persisted state, without relying on in-memory deduplication.
	pool, err := pgxpool.NewWithConfig(ctx, r.DB.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if credited, err := NewRepository(pool).CreditDeposit(ctx, 1); err != nil || credited {
		t.Fatalf("retry credit=%v err=%v", credited, err)
	}
	assertCreditTotals(t, r, ctx, 1, "1000000")
	ids, err = r.LoadConfirmedDepositIDs(ctx, 10)
	if err != nil || len(ids) != 0 {
		t.Fatalf("credited deposit still pending: %v err=%v", ids, err)
	}
}

func TestCreditDepositRejectsUnconfirmed(t *testing.T) {
	r, ctx := creditTestRepository(t)
	for i, status := range []DepositStatus{DepositStatusDetected, DepositStatusReverted} {
		id := int64(i + 1)
		insertCreditTestDeposit(t, r, ctx, id, status)
		if credited, err := r.CreditDeposit(ctx, id); err == nil || credited {
			t.Fatalf("credited %s deposit: credited=%v err=%v", status, credited, err)
		}
	}
	if credited, err := r.CreditDeposit(ctx, 999); err == nil || credited {
		t.Fatalf("credited nonexistent deposit: credited=%v err=%v", credited, err)
	}
	assertCreditTotals(t, r, ctx, 0, "0")
}

func TestCreditDepositConcurrent(t *testing.T) {
	r, ctx := creditTestRepository(t)
	const deposits = 12
	for id := int64(1); id <= deposits; id++ {
		insertCreditTestDeposit(t, r, ctx, id, DepositStatusConfirmed)
	}
	type result struct {
		credited bool
		err      error
	}
	results := make(chan result, deposits*3)
	var wg sync.WaitGroup
	for id := int64(1); id <= deposits; id++ {
		for retry := 0; retry < 3; retry++ {
			wg.Add(1)
			go func(depositID int64) {
				defer wg.Done()
				credited, err := r.CreditDeposit(ctx, depositID)
				results <- result{credited, err}
			}(id)
		}
	}
	wg.Wait()
	close(results)
	credits := 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.credited {
			credits++
		}
	}
	if credits != deposits {
		t.Fatalf("successful credits=%d want %d", credits, deposits)
	}
	assertCreditTotals(t, r, ctx, deposits, "12000000")
	var creditedCount int
	if err := r.DB.QueryRow(ctx, "SELECT count(*) FROM deposits WHERE status = 'credited'").Scan(&creditedCount); err != nil {
		t.Fatal(err)
	}
	if creditedCount != deposits {
		t.Fatalf("credited statuses=%d want %d", creditedCount, deposits)
	}
}

func TestCreditDepositRollsBackEveryWriteFailure(t *testing.T) {
	for _, table := range []string{"ledger_transactions", "ledger", "balances", "deposits"} {
		t.Run(table, func(t *testing.T) {
			r, ctx := creditTestRepository(t)
			insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
			events := "INSERT OR UPDATE"
			if table == "balances" {
				// Fail the increase after journal entries, not the initial zero row.
				events = "UPDATE"
			}
			_, err := r.DB.Exec(ctx, `
				CREATE FUNCTION fail_credit() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'injected credit failure'; END $$;
				CREATE TRIGGER fail_credit BEFORE `+events+` ON `+table+`
				FOR EACH ROW EXECUTE FUNCTION fail_credit();
			`)
			if err != nil {
				t.Fatal(err)
			}
			if credited, err := r.CreditDeposit(ctx, 1); err == nil || credited {
				t.Fatalf("failure did not abort credit: credited=%v err=%v", credited, err)
			}
			assertCreditTotals(t, r, ctx, 0, "0")
			var status DepositStatus
			if err := r.DB.QueryRow(ctx, "SELECT status FROM deposits WHERE id = 1").Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != DepositStatusConfirmed {
				t.Fatalf("status after rollback=%s", status)
			}
			if _, err := r.DB.Exec(ctx, "DROP TRIGGER fail_credit ON "+table); err != nil {
				t.Fatal(err)
			}
			if credited, err := r.CreditDeposit(ctx, 1); err != nil || !credited {
				t.Fatalf("retry after failure: credited=%v err=%v", credited, err)
			}
			assertCreditTotals(t, r, ctx, 1, "1000000")
		})
	}
}
