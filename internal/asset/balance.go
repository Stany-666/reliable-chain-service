package asset

import (
	"context"
	"fmt"
	"math/big"
)

// UserBalance uses asset integer units; Total is Available + Frozen.
type UserBalance struct {
	Available *big.Int
	Frozen    *big.Int
}

func (r *Repository) GetUserBalance(ctx context.Context, userID, assetID int64) (UserBalance, error) {
	var available, frozen string
	err := r.DB.QueryRow(ctx, `
		SELECT coalesce(sum(b.amount) FILTER (WHERE a.account_type = 'available'), 0)::text,
		       coalesce(sum(b.amount) FILTER (WHERE a.account_type = 'frozen'), 0)::text
		FROM accounts a JOIN balances b ON b.account_id = a.id
		WHERE a.user_id = $1 AND a.asset_id = $2
	`, userID, assetID).Scan(&available, &frozen)
	if err != nil {
		return UserBalance{}, fmt.Errorf("load user balance: %w", err)
	}

	result := UserBalance{Available: new(big.Int), Frozen: new(big.Int)}

	if _, ok := result.Available.SetString(available, 10); !ok {
		return UserBalance{}, fmt.Errorf("invalid available balance")
	}

	if _, ok := result.Frozen.SetString(frozen, 10); !ok {
		return UserBalance{}, fmt.Errorf("invalid frozen balance")
	}
	return result, nil
}

// GetCustodyBalance returns the amount of the custody account.
func (r *Repository) GetCustodyBalance(ctx context.Context) (amount string, err error) {
	err = r.DB.QueryRow(ctx, `
		SELECT amount::text
		FROM balances
		WHERE account_id = 1
	`).Scan(&amount)
	if err != nil {
		return "", fmt.Errorf("load custody balance: %w", err)
	}
	return amount, nil
}
