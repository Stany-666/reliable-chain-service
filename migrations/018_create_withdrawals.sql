CREATE TABLE withdrawals (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    asset_id   BIGINT NOT NULL,
    amount     NUMERIC NOT NULL,
    to_address TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
