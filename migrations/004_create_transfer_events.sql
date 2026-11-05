CREATE TABLE transfer_events (
    id BIGSERIAL PRIMARY KEY,

    tx_hash TEXT NOT NULL,
    log_index BIGINT NOT NULL,

    block_number BIGINT NOT NULL,

    token_address TEXT NOT NULL,

    from_address TEXT NOT NULL,
    to_address TEXT NOT NULL,

    amount NUMERIC NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(tx_hash, log_index)
);