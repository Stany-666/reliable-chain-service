-- Run the entire file in one transaction (psql --single-transaction).
-- Abort rather than convert inconsistent single-entry data.
LOCK TABLE deposits, ledger, balances IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM deposits d FULL JOIN ledger l ON l.deposit_id = d.id
        WHERE (d.status = 'credited' AND l.id IS NULL)
           OR (l.id IS NOT NULL AND (
               d.status IS DISTINCT FROM 'credited'
               OR d.user_id IS DISTINCT FROM l.user_id
               OR d.asset_id IS DISTINCT FROM l.asset_id
               OR d.amount IS DISTINCT FROM l.amount
           ))
    ) THEN
        RAISE EXCEPTION 'Cannot migrate: deposits and ledger disagree';
    END IF;
    IF EXISTS (
        WITH expected AS (
            SELECT user_id, asset_id, sum(amount) AS amount
            FROM ledger GROUP BY user_id, asset_id
        )
        SELECT 1 FROM expected e FULL JOIN balances b USING (user_id, asset_id)
        WHERE coalesce(e.amount, 0) <> coalesce(b.amount, 0)
    ) THEN
        RAISE EXCEPTION 'Cannot migrate: ledger and balances disagree';
    END IF;
END $$;

ALTER TABLE ledger RENAME TO ledger_legacy;
ALTER TABLE ledger_legacy RENAME CONSTRAINT ledger_pkey TO ledger_legacy_pkey;
ALTER TABLE ledger_legacy RENAME CONSTRAINT ledger_deposit_id_key TO ledger_legacy_deposit_id_key;
ALTER SEQUENCE ledger_id_seq RENAME TO ledger_legacy_id_seq;
ALTER TABLE balances RENAME TO balances_legacy;
ALTER TABLE balances_legacy RENAME CONSTRAINT balances_pkey TO balances_legacy_pkey;

CREATE TABLE accounts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id),
    asset_id BIGINT NOT NULL REFERENCES assets(id),
    account_type TEXT NOT NULL
        CHECK (account_type IN ('custody', 'available', 'frozen')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((account_type = 'custody' AND user_id IS NULL)
        OR (account_type IN ('available', 'frozen') AND user_id IS NOT NULL)),
    UNIQUE NULLS NOT DISTINCT (user_id, asset_id, account_type),
    UNIQUE (id, asset_id)
);

CREATE TABLE ledger_transactions (
    id BIGSERIAL PRIMARY KEY,
    asset_id BIGINT NOT NULL REFERENCES assets(id),
    business_type TEXT NOT NULL
        CHECK (business_type IN ('deposit', 'freeze', 'unfreeze', 'withdrawal', 'reversal')),
    business_id BIGINT NOT NULL CHECK (business_id > 0),
    deposit_id BIGINT UNIQUE REFERENCES deposits(id),
    reverses_transaction_id BIGINT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (business_type, business_id),
    UNIQUE (id, asset_id),
    FOREIGN KEY (reverses_transaction_id, asset_id)
        REFERENCES ledger_transactions(id, asset_id),
    CHECK ((business_type = 'deposit' AND deposit_id IS NOT NULL AND deposit_id = business_id)
        OR (business_type <> 'deposit' AND deposit_id IS NULL)),
    CHECK ((business_type = 'reversal') = (reverses_transaction_id IS NOT NULL)),
    CHECK (reverses_transaction_id IS NULL OR reverses_transaction_id <> id)
);

CREATE TABLE ledger (
    id BIGSERIAL PRIMARY KEY,
    transaction_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    asset_id BIGINT NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount NUMERIC NOT NULL CHECK (amount > 0 AND amount = trunc(amount)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (transaction_id, asset_id) REFERENCES ledger_transactions(id, asset_id),
    FOREIGN KEY (account_id, asset_id) REFERENCES accounts(id, asset_id),
    UNIQUE (transaction_id, account_id, direction)
);
CREATE INDEX ledger_account_id_idx ON ledger(account_id);

CREATE TABLE balances (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id),
    amount NUMERIC NOT NULL DEFAULT 0
        CHECK (amount >= 0 AND amount = trunc(amount)),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Existing assets get a custody account; every existing user gets both buckets.
INSERT INTO accounts (asset_id, account_type)
SELECT id, 'custody' FROM assets ORDER BY id;
INSERT INTO accounts (user_id, asset_id, account_type)
SELECT u.id, a.id, bucket.account_type
FROM users u CROSS JOIN assets a
CROSS JOIN (VALUES ('available'), ('frozen')) AS bucket(account_type)
ORDER BY u.id, a.id, bucket.account_type;

INSERT INTO ledger_transactions (id, asset_id, business_type, business_id, deposit_id, created_at)
SELECT id, asset_id, 'deposit', deposit_id, deposit_id, created_at
FROM ledger_legacy ORDER BY id;
SELECT setval(pg_get_serial_sequence('ledger_transactions', 'id'),
              coalesce(max(id), 1), max(id) IS NOT NULL)
FROM ledger_transactions;

INSERT INTO ledger (transaction_id, account_id, asset_id, direction, amount, created_at)
SELECT l.id, a.id, l.asset_id, 'debit', l.amount, l.created_at
FROM ledger_legacy l JOIN accounts a
  ON a.asset_id = l.asset_id AND a.account_type = 'custody'
UNION ALL
SELECT l.id, a.id, l.asset_id, 'credit', l.amount, l.created_at
FROM ledger_legacy l JOIN accounts a
  ON a.asset_id = l.asset_id AND a.user_id = l.user_id AND a.account_type = 'available';

-- Custody is an asset (debit increases); available/frozen are user liabilities
-- (credit increases). Frozen starts at zero; old user balances are preserved.
INSERT INTO balances (account_id, amount)
SELECT a.id, coalesce(sum(CASE
    WHEN (a.account_type = 'custody' AND l.direction = 'debit')
      OR (a.account_type <> 'custody' AND l.direction = 'credit') THEN l.amount
    ELSE -l.amount END), 0)
FROM accounts a LEFT JOIN ledger l ON l.account_id = a.id
GROUP BY a.id;

-- Protect posted history and account identity. Corrections need new entries.
CREATE FUNCTION reject_accounting_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is immutable; write a new transaction instead', TG_TABLE_NAME;
END $$;
CREATE TRIGGER immutable_ledger BEFORE UPDATE OR DELETE OR TRUNCATE ON ledger
FOR EACH STATEMENT EXECUTE FUNCTION reject_accounting_mutation();
CREATE TRIGGER immutable_transactions BEFORE UPDATE OR DELETE OR TRUNCATE ON ledger_transactions
FOR EACH STATEMENT EXECUTE FUNCTION reject_accounting_mutation();
CREATE TRIGGER immutable_accounts BEFORE UPDATE OR DELETE OR TRUNCATE ON accounts
FOR EACH STATEMENT EXECUTE FUNCTION reject_accounting_mutation();
CREATE TRIGGER protect_balance_truncate BEFORE TRUNCATE ON balances
FOR EACH STATEMENT EXECUTE FUNCTION reject_accounting_mutation();
CREATE TRIGGER immutable_legacy_ledger BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON ledger_legacy
FOR EACH STATEMENT EXECUTE FUNCTION reject_accounting_mutation();
CREATE TRIGGER immutable_legacy_balances BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON balances_legacy
FOR EACH STATEMENT EXECUTE FUNCTION reject_accounting_mutation();

-- Serialize postings to the same account, including direct SQL writes.
CREATE FUNCTION lock_posting_account() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM 1 FROM accounts WHERE id = NEW.account_id FOR UPDATE;
    RETURN NEW;
END $$;
CREATE TRIGGER lock_posting_account BEFORE INSERT ON ledger
FOR EACH ROW EXECUTE FUNCTION lock_posting_account();

CREATE FUNCTION check_ledger_transaction(checked_id BIGINT) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    journal ledger_transactions%ROWTYPE;
    deposit deposits%ROWTYPE;
    entry_count BIGINT;
    debit_amount NUMERIC;
    credit_amount NUMERIC;
BEGIN
    SELECT * INTO STRICT journal FROM ledger_transactions WHERE id = checked_id;
    SELECT count(*), coalesce(sum(amount) FILTER (WHERE direction = 'debit'), 0),
           coalesce(sum(amount) FILTER (WHERE direction = 'credit'), 0)
    INTO entry_count, debit_amount, credit_amount
    FROM ledger WHERE transaction_id = checked_id;
    IF entry_count < 2 OR debit_amount <> credit_amount THEN
        RAISE EXCEPTION 'Transaction % is not balanced', checked_id;
    END IF;
    IF journal.business_type = 'deposit' THEN
        SELECT * INTO STRICT deposit FROM deposits WHERE id = journal.deposit_id;
        IF deposit.status <> 'credited' OR deposit.asset_id <> journal.asset_id
           OR entry_count <> 2 OR debit_amount <> deposit.amount
           OR NOT EXISTS (
               SELECT 1 FROM ledger l JOIN accounts a ON a.id = l.account_id
               WHERE l.transaction_id = checked_id AND l.direction = 'debit'
                 AND a.account_type = 'custody'
           ) OR NOT EXISTS (
               SELECT 1 FROM ledger l JOIN accounts a ON a.id = l.account_id
               WHERE l.transaction_id = checked_id AND l.direction = 'credit'
                 AND a.account_type = 'available' AND a.user_id = deposit.user_id
           ) THEN
            RAISE EXCEPTION 'Transaction % does not match its deposit', checked_id;
        END IF;
    END IF;
END $$;

CREATE FUNCTION check_account_balance(checked_id BIGINT) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    expected_amount NUMERIC;
    actual_amount NUMERIC;
BEGIN
    SELECT coalesce(sum(CASE
        WHEN (a.account_type = 'custody' AND l.direction = 'debit')
          OR (a.account_type <> 'custody' AND l.direction = 'credit') THEN l.amount
        ELSE -l.amount END), 0)
    INTO expected_amount
    FROM accounts a LEFT JOIN ledger l ON l.account_id = a.id
    WHERE a.id = checked_id;
    SELECT amount INTO actual_amount FROM balances WHERE account_id = checked_id;
    IF actual_amount IS NULL OR actual_amount <> expected_amount THEN
        RAISE EXCEPTION 'Account % balance does not match ledger', checked_id;
    END IF;
END $$;

CREATE FUNCTION validate_posted_transaction() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_TABLE_NAME = 'ledger_transactions' THEN
        PERFORM check_ledger_transaction(NEW.id);
    ELSE
        PERFORM check_ledger_transaction(NEW.transaction_id);
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER balanced_transaction AFTER INSERT ON ledger_transactions
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_posted_transaction();
CREATE CONSTRAINT TRIGGER balanced_entries AFTER INSERT ON ledger
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_posted_transaction();

CREATE FUNCTION validate_account_balance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM check_account_balance(coalesce(NEW.account_id, OLD.account_id));
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER consistent_balance AFTER INSERT OR UPDATE OR DELETE ON balances
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_account_balance();
CREATE CONSTRAINT TRIGGER consistent_entries AFTER INSERT ON ledger
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_account_balance();
CREATE FUNCTION validate_new_account() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM check_account_balance(NEW.id);
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER initialized_account AFTER INSERT ON accounts
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_new_account();

CREATE FUNCTION protect_credited_deposit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status = 'credited' AND (
        NEW.status IS DISTINCT FROM OLD.status OR NEW.amount IS DISTINCT FROM OLD.amount
        OR NEW.user_id IS DISTINCT FROM OLD.user_id OR NEW.asset_id IS DISTINCT FROM OLD.asset_id
        OR NEW.transfer_event_id IS DISTINCT FROM OLD.transfer_event_id
    ) THEN
        RAISE EXCEPTION 'Credited deposit % is immutable', OLD.id;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER protect_credited_deposit BEFORE UPDATE ON deposits
FOR EACH ROW EXECUTE FUNCTION protect_credited_deposit();
CREATE FUNCTION validate_credited_deposit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    journal_id BIGINT;
BEGIN
    IF NEW.status = 'credited' THEN
        SELECT id INTO journal_id FROM ledger_transactions WHERE deposit_id = NEW.id;
        IF journal_id IS NULL THEN
            RAISE EXCEPTION 'Credited deposit % has no journal', NEW.id;
        END IF;
        PERFORM check_ledger_transaction(journal_id);
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER credited_deposit_has_journal AFTER INSERT OR UPDATE ON deposits
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_credited_deposit();

-- Validate converted data before committing; any mismatch rolls back all DDL.
DO $$
DECLARE
    checked_id BIGINT;
BEGIN
    FOR checked_id IN SELECT id FROM ledger_transactions LOOP
        PERFORM check_ledger_transaction(checked_id);
    END LOOP;
    FOR checked_id IN SELECT id FROM accounts LOOP
        PERFORM check_account_balance(checked_id);
    END LOOP;
    IF EXISTS (
        SELECT 1 FROM balances_legacy old
        JOIN accounts a ON a.user_id = old.user_id AND a.asset_id = old.asset_id
                       AND a.account_type = 'available'
        JOIN balances b ON b.account_id = a.id
        WHERE old.amount <> b.amount
    ) THEN
        RAISE EXCEPTION 'Converted available balance differs from legacy balance';
    END IF;
END $$;
