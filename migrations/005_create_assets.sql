CREATE TABLE assets (
    id BIGSERIAL PRIMARY KEY,

    chain_id BIGINT NOT NULL,
    symbol TEXT NOT NULL,
    token_address TEXT NOT NULL,
    decimals SMALLINT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(chain_id, token_address),
    CHECK(decimals >= 0)
);