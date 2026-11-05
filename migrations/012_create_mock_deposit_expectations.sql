CREATE TABLE mock_deposit_expectations (
    id BIGSERIAL PRIMARY KEY,
    test_case TEXT NOT NULL UNIQUE,
    chain_id BIGINT NOT NULL,
    asset_id BIGINT NOT NULL REFERENCES assets(id),
    address TEXT,
    user_id BIGINT,
    transfer_event_ids BIGINT[] NOT NULL,
    transfer_count BIGINT NOT NULL CHECK (transfer_count >= 0),
    expected_deposit_count BIGINT NOT NULL
        CHECK (expected_deposit_count >= 0),
    expected_amount NUMERIC(78, 0) NOT NULL CHECK (expected_amount >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CHECK (transfer_count = cardinality(transfer_event_ids)),
    CHECK (expected_deposit_count <= transfer_count),
    CHECK ((address IS NULL) = (user_id IS NULL))
);
