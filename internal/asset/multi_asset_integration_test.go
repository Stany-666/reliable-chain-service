package asset

import (
	"sync"
	"testing"
)

func TestCreditDepositAcrossUsersAndAssets(t *testing.T) {
	r, ctx := creditTestRepository(t)
	_, err := r.DB.Exec(ctx, `
		INSERT INTO users (id, email) VALUES (2, 'second-user@example.test');
		INSERT INTO assets (id, chain_id, symbol, token_address, decimals)
		VALUES (2, 1, 'USDC', '0xsecond-token', 6);
	`)
	if err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 4; id++ {
		insertCreditTestDeposit(t, r, ctx, id, DepositStatusConfirmed)
		userID, assetID := (id-1)%2+1, (id-1)/2+1
		if _, err := r.DB.Exec(ctx, "UPDATE deposits SET user_id = $2, asset_id = $3 WHERE id = $1", id, userID, assetID); err != nil {
			t.Fatal(err)
		}
		if assetID == 2 {
			if _, err := r.DB.Exec(ctx, "UPDATE mock_transfer_events SET token_address = '0xsecond-token' WHERE id = $1", id); err != nil {
				t.Fatal(err)
			}
		}
	}
	var wg sync.WaitGroup
	errors := make(chan error, 12)
	for id := int64(1); id <= 4; id++ {
		for retry := 0; retry < 3; retry++ {
			wg.Add(1)
			go func(id int64) {
				defer wg.Done()
				_, err := r.CreditDeposit(ctx, id)
				errors <- err
			}(id)
		}
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for userID := int64(1); userID <= 2; userID++ {
		for assetID := int64(1); assetID <= 2; assetID++ {
			balance, err := r.GetUserBalance(ctx, userID, assetID)
			if err != nil || balance.Available.String() != "1000000" || balance.Frozen.Sign() != 0 {
				t.Fatalf("user=%d asset=%d balance=%v err=%v", userID, assetID, balance, err)
			}
		}
	}
	var custodyCount int
	var minAmount, maxAmount string
	err = r.DB.QueryRow(ctx, `
		SELECT count(*), min(b.amount)::text, max(b.amount)::text
		FROM accounts a JOIN balances b ON b.account_id = a.id
		WHERE a.account_type = 'custody'
	`).Scan(&custodyCount, &minAmount, &maxAmount)
	if err != nil || custodyCount != 2 || minAmount != "2000000" || maxAmount != "2000000" {
		t.Fatalf("custody accounts=%d range=%s..%s err=%v", custodyCount, minAmount, maxAmount, err)
	}
}
