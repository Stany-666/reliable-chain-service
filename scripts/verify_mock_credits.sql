-- This fixture has deposits only: frozen is zero; custody equals user liabilities.
-- Every user's passed should be true; both mismatch counts should be zero.
WITH expected AS (
    SELECT user_id, asset_id,
           sum(expected_deposit_count) AS deposit_count,
           sum(expected_amount) AS amount
    FROM mock_deposit_expectations
    WHERE user_id IS NOT NULL
    GROUP BY user_id, asset_id
), user_balances AS (
    SELECT a.user_id, a.asset_id, count(*) AS account_count,
           coalesce(sum(b.amount) FILTER (WHERE a.account_type = 'available'), 0) AS available,
           coalesce(sum(b.amount) FILTER (WHERE a.account_type = 'frozen'), 0) AS frozen
    FROM accounts a JOIN balances b ON b.account_id = a.id
    WHERE a.user_id IS NOT NULL
    GROUP BY a.user_id, a.asset_id
), postings AS (
    SELECT d.user_id, d.asset_id, count(DISTINCT j.id) AS journals,
           count(l.id) AS entries,
           coalesce(sum(l.amount) FILTER (WHERE l.direction = 'credit'), 0) AS credit_amount
    FROM deposits d JOIN ledger_transactions j ON j.deposit_id = d.id
    JOIN ledger l ON l.transaction_id = j.id
    GROUP BY d.user_id, d.asset_id
), actual AS (
    SELECT e.*, coalesce(p.journals, 0) AS journals, coalesce(p.entries, 0) AS entries,
           coalesce(p.credit_amount, 0) AS credit_amount,
           coalesce(b.account_count, 0) AS account_count,
           coalesce(b.available, 0) AS available, coalesce(b.frozen, 0) AS frozen,
           (SELECT count(*) FROM deposits d
            WHERE d.user_id = e.user_id AND d.asset_id = e.asset_id
              AND d.status = 'credited') AS credited_count
    FROM expected e
    LEFT JOIN user_balances b USING (user_id, asset_id)
    LEFT JOIN postings p USING (user_id, asset_id)
)
SELECT user_id, asset_id, deposit_count AS expected_deposits,
       journals, entries, credited_count, amount AS expected_amount, available, frozen,
       account_count = 2 AND journals = deposit_count AND entries = 2 * deposit_count
           AND credited_count = deposit_count AND credit_amount = amount
           AND available = amount AND frozen = 0 AS passed
FROM actual ORDER BY user_id, asset_id;

SELECT count(*) AS unbalanced_transactions FROM (
    SELECT j.id FROM ledger_transactions j LEFT JOIN ledger l ON l.transaction_id = j.id
    GROUP BY j.id
    HAVING count(l.id) < 2
        OR coalesce(sum(l.amount) FILTER (WHERE l.direction = 'debit'), 0)
           <> coalesce(sum(l.amount) FILTER (WHERE l.direction = 'credit'), 0)
) invalid;

WITH account_totals AS (
    SELECT a.id, a.asset_id, a.account_type, b.amount,
           coalesce(sum(CASE
               WHEN (a.account_type = 'custody' AND l.direction = 'debit')
                 OR (a.account_type <> 'custody' AND l.direction = 'credit') THEN l.amount
               ELSE -l.amount END), 0) AS ledger_balance
    FROM accounts a LEFT JOIN balances b ON b.account_id = a.id
    LEFT JOIN ledger l ON l.account_id = a.id
    GROUP BY a.id, b.amount
)
SELECT asset_id,
       count(*) FILTER (WHERE amount IS DISTINCT FROM ledger_balance) AS balance_mismatches,
       sum(amount) FILTER (WHERE account_type = 'custody') AS custody,
       sum(amount) FILTER (WHERE account_type = 'available') AS available,
       sum(amount) FILTER (WHERE account_type = 'frozen') AS frozen,
       sum(amount) FILTER (WHERE account_type = 'custody')
           = sum(amount) FILTER (WHERE account_type IN ('available', 'frozen')) AS matched
FROM account_totals GROUP BY asset_id ORDER BY asset_id;
