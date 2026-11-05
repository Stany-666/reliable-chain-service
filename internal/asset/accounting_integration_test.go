package asset

import (
	"context"
	"os"
	"testing"
)

func applyDoubleEntryMigration(r *Repository, ctx context.Context) error {
	sql, err := os.ReadFile("../../migrations/015_create_double_entry_ledger.sql")
	if err != nil {
		return err
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func seedLegacyCredit(t *testing.T, r *Repository, ctx context.Context) {
	t.Helper()
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	_, err := r.DB.Exec(ctx, `
		INSERT INTO ledger (deposit_id, user_id, asset_id, amount) VALUES (1, 1, 1, 1000000);
		INSERT INTO balances (user_id, asset_id, amount) VALUES (1, 1, 1000000);
		UPDATE deposits SET status = 'credited' WHERE id = 1;
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAccountingMigrationPreservesBalances(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "014_create_balances.sql")
	seedLegacyCredit(t, r, ctx)
	insertCreditTestDeposit(t, r, ctx, 2, DepositStatusConfirmed)
	if err := applyDoubleEntryMigration(r, ctx); err != nil {
		t.Fatal(err)
	}
	assertCreditTotals(t, r, ctx, 1, "1000000")
	var legacyCount, accountCount int
	if err := r.DB.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM ledger_legacy), (SELECT count(*) FROM accounts)
	`).Scan(&legacyCount, &accountCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 1 || accountCount != 3 {
		t.Fatalf("legacy=%d accounts=%d", legacyCount, accountCount)
	}
	if credited, err := r.CreditDeposit(ctx, 1); err != nil || credited {
		t.Fatalf("migrated deposit was re-credited: %v %v", credited, err)
	}
	if credited, err := r.CreditDeposit(ctx, 2); err != nil || !credited {
		t.Fatalf("new credit after migration: %v %v", credited, err)
	}
	assertCreditTotals(t, r, ctx, 2, "2000000")
	if _, err := r.DB.Exec(ctx, "UPDATE ledger_legacy SET amount = amount"); err == nil {
		t.Fatal("legacy history was writable")
	}
}

func TestAccountingMigrationRejectsInconsistentData(t *testing.T) {
	for _, corruptSQL := range []string{
		"UPDATE balances SET amount = 999999",
		"UPDATE deposits SET status = 'confirmed'",
	} {
		t.Run(corruptSQL, func(t *testing.T) {
			r, ctx := creditTestRepositoryThrough(t, "014_create_balances.sql")
			seedLegacyCredit(t, r, ctx)
			if _, err := r.DB.Exec(ctx, corruptSQL); err != nil {
				t.Fatal(err)
			}
			if err := applyDoubleEntryMigration(r, ctx); err == nil {
				t.Fatal("inconsistent legacy data was migrated")
			}
			var unchanged bool
			if err := r.DB.QueryRow(ctx, `
				SELECT to_regclass('accounts') IS NULL AND to_regclass('ledger_legacy') IS NULL
			`).Scan(&unchanged); err != nil || !unchanged {
				t.Fatalf("failed migration changed schema: unchanged=%v err=%v", unchanged, err)
			}
		})
	}
}

func TestAccountingDropLegacyPreservesCredits(t *testing.T) {
	r, ctx := creditTestRepositoryThrough(t, "014_create_balances.sql")
	seedLegacyCredit(t, r, ctx)
	if err := applyDoubleEntryMigration(r, ctx); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../migrations/016_drop_legacy_accounting_tables.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var removed bool
	if err := r.DB.QueryRow(ctx, `
		SELECT to_regclass('ledger_legacy') IS NULL AND to_regclass('balances_legacy') IS NULL
	`).Scan(&removed); err != nil || !removed {
		t.Fatalf("legacy tables removed=%v err=%v", removed, err)
	}
	assertCreditTotals(t, r, ctx, 1, "1000000")
	if credited, err := r.CreditDeposit(ctx, 1); err != nil || credited {
		t.Fatalf("migrated deposit was re-credited: %v %v", credited, err)
	}
	insertCreditTestDeposit(t, r, ctx, 2, DepositStatusConfirmed)
	if credited, err := r.CreditDeposit(ctx, 2); err != nil || !credited {
		t.Fatalf("credit after removing legacy tables: %v %v", credited, err)
	}
	assertCreditTotals(t, r, ctx, 2, "2000000")
}

func testPostingAccount(t *testing.T, r *Repository, ctx context.Context, userID int64, accountType string) int64 {
	t.Helper()
	var id int64
	err := r.DB.QueryRow(ctx, `
		SELECT id FROM accounts WHERE asset_id = 1 AND account_type = $1
		AND (user_id = $2 OR account_type = 'custody')
	`, accountType, userID).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testFreezePosting(t *testing.T, r *Repository, ctx context.Context, debit, credit string) error {
	t.Helper()
	availableID := testPostingAccount(t, r, ctx, 1, "available")
	frozenID := testPostingAccount(t, r, ctx, 1, "frozen")
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var journalID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO ledger_transactions (asset_id, business_type, business_id)
		VALUES (1, 'freeze', 1) RETURNING id
	`).Scan(&journalID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger (transaction_id, account_id, asset_id, direction, amount)
		VALUES ($1, $2, 1, 'debit', $4), ($1, $3, 1, 'credit', $5)
	`, journalID, availableID, frozenID, debit, credit)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE balances SET amount = amount - $2::numeric WHERE account_id = $1", availableID, debit); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE balances SET amount = amount + $2::numeric WHERE account_id = $1", frozenID, credit); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func TestAccountingAvailableAndFrozen(t *testing.T) {
	r, ctx := creditTestRepository(t)
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// This SQL fixture exercises both liability buckets, not a withdrawal service.
	if err := testFreezePosting(t, r, ctx, "300000", "300000"); err != nil {
		t.Fatal(err)
	}
	balance, err := r.GetUserBalance(ctx, 1, 1)
	if err != nil || balance.Available.String() != "700000" || balance.Frozen.String() != "300000" {
		t.Fatalf("available/frozen=%v err=%v", balance, err)
	}
	var custody string
	err = r.DB.QueryRow(ctx, `
		SELECT b.amount::text FROM balances b JOIN accounts a ON a.id = b.account_id
		WHERE a.account_type = 'custody'
	`).Scan(&custody)
	if err != nil || custody != "1000000" {
		t.Fatalf("custody changed on freeze: %s %v", custody, err)
	}
}

func TestAccountingRejectsUnbalancedAndOverdrawn(t *testing.T) {
	for _, amounts := range [][2]string{{"300000", "299999"}, {"1000001", "1000001"}} {
		t.Run(amounts[0]+"_"+amounts[1], func(t *testing.T) {
			r, ctx := creditTestRepository(t)
			insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
			if _, err := r.CreditDeposit(ctx, 1); err != nil {
				t.Fatal(err)
			}
			if err := testFreezePosting(t, r, ctx, amounts[0], amounts[1]); err == nil {
				t.Fatal("invalid posting committed")
			}
			assertCreditTotals(t, r, ctx, 1, "1000000")
		})
	}
}

func TestAccountingRejectsDirectTampering(t *testing.T) {
	r, ctx := creditTestRepository(t)
	insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
	if _, err := r.CreditDeposit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"UPDATE balances SET amount = amount + 1",
		"DELETE FROM balances",
		"TRUNCATE balances",
		"UPDATE ledger SET amount = amount",
		"DELETE FROM ledger",
		"UPDATE ledger_transactions SET business_id = business_id",
		"UPDATE accounts SET account_type = account_type",
		"UPDATE deposits SET user_id = 999 WHERE id = 1",
		"INSERT INTO ledger_transactions (asset_id, business_type, business_id) VALUES (1, 'freeze', 9)",
	} {
		if _, err := r.DB.Exec(ctx, sql); err == nil {
			t.Errorf("invalid direct write succeeded: %s", sql)
		}
	}
	assertCreditTotals(t, r, ctx, 1, "1000000")
}

func TestAccountingRejectsWrongDepositAccountAndAsset(t *testing.T) {
	for _, wrong := range []string{"owner", "bucket", "asset"} {
		t.Run(wrong, func(t *testing.T) {
			r, ctx := creditTestRepository(t)
			insertCreditTestDeposit(t, r, ctx, 1, DepositStatusConfirmed)
			if _, err := r.CreditDeposit(ctx, 1); err != nil {
				t.Fatal(err)
			}
			insertCreditTestDeposit(t, r, ctx, 2, DepositStatusConfirmed)
			tx, err := r.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			custodyID := testPostingAccount(t, r, ctx, 1, "custody")
			creditID := testPostingAccount(t, r, ctx, 1, "available")
			if wrong == "bucket" {
				creditID = testPostingAccount(t, r, ctx, 1, "frozen")
			}
			if wrong == "owner" {
				_, err = tx.Exec(ctx, "INSERT INTO users (id, email) VALUES (2, 'wrong-owner@example.test')")
				if err == nil {
					err = tx.QueryRow(ctx, `
						INSERT INTO accounts (user_id, asset_id, account_type)
						VALUES (2, 1, 'available') RETURNING id
					`).Scan(&creditID)
				}
				if err == nil {
					_, err = tx.Exec(ctx, "INSERT INTO balances (account_id) VALUES ($1)", creditID)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			var journalID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO ledger_transactions (asset_id, business_type, business_id, deposit_id)
				VALUES (1, 'deposit', 2, 2) RETURNING id
			`).Scan(&journalID); err != nil {
				t.Fatal(err)
			}
			assetID := int64(1)
			if wrong == "asset" {
				assetID = 999
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO ledger (transaction_id, account_id, asset_id, direction, amount)
				VALUES ($1, $2, $4, 'debit', 1000000), ($1, $3, $4, 'credit', 1000000)
			`, journalID, custodyID, creditID, assetID)
			if err == nil {
				_, err = tx.Exec(ctx, `
					UPDATE balances SET amount = amount + 1000000 WHERE account_id = ANY($1::bigint[])
				`, []int64{custodyID, creditID})
			}
			if err == nil {
				_, err = tx.Exec(ctx, "UPDATE deposits SET status = 'credited' WHERE id = 2")
			}
			if err == nil {
				err = tx.Commit(ctx)
			} else {
				tx.Rollback(ctx)
			}
			if err == nil {
				t.Fatal("posting with incorrect account or asset committed")
			}
			assertCreditTotals(t, r, ctx, 1, "1000000")
		})
	}
}
