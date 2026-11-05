package asset

import "context"

// 发生退回时，需要将已入账的金额退回给用户
func (r *Repository) ReverseCreditedDeposit(ctx context.Context, depositID string) error {

	return nil
}
