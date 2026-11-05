CREATE TABLE deposit_addresses (
    id BIGSERIAL PRIMARY KEY,

    user_id BIGINT NOT NULL REFERENCES users(id),
    chain_id BIGINT NOT NULL,
    address TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(chain_id, address)
);
