CREATE TABLE mock_transfer_events (
    id BIGSERIAL PRIMARY KEY,

    chain_id BIGINT NOT NULL,
    tx_hash TEXT NOT NULL,
    log_index BIGINT NOT NULL,

    block_number BIGINT NOT NULL,
    block_hash TEXT NOT NULL,

    token_address TEXT NOT NULL,
    from_address TEXT NOT NULL,
    to_address TEXT NOT NULL,
    amount NUMERIC(78, 0) NOT NULL CHECK (amount >= 0),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (chain_id, tx_hash, log_index)
);
