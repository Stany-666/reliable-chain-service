-- One entry per credited deposit. Amounts use the asset's smallest unit.
CREATE TABLE ledger (
    id BIGSERIAL PRIMARY KEY,
    deposit_id BIGINT NOT NULL UNIQUE REFERENCES deposits(id),
    user_id BIGINT NOT NULL REFERENCES users(id),
    asset_id BIGINT NOT NULL REFERENCES assets(id),
    amount NUMERIC NOT NULL CHECK (amount > 0 AND amount = trunc(amount)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
