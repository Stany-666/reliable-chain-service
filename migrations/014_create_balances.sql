-- A user has one balance per asset. Use integer units, never floating point.
CREATE TABLE balances (
    user_id BIGINT NOT NULL REFERENCES users(id),
    asset_id BIGINT NOT NULL REFERENCES assets(id),
    amount NUMERIC NOT NULL DEFAULT 0
        CHECK (amount >= 0 AND amount = trunc(amount)),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_id, asset_id)
);
