CREATE TABLE deposits (
    id BIGSERIAL PRIMARY KEY,

    transfer_event_id BIGINT NOT NULL
        REFERENCES transfer_events(id),

    user_id BIGINT NOT NULL
        REFERENCES users(id),

    asset_id BIGINT NOT NULL
        REFERENCES assets(id),

    amount NUMERIC NOT NULL
        CHECK (amount > 0),

    status TEXT NOT NULL DEFAULT 'detected'
        CHECK (status IN (
            'detected',
            'confirmed',
            'credited',
            'reverted'
        )),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(transfer_event_id)
);
