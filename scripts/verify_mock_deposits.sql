-- Compare recognition results with the saved baseline, independently of status.
WITH actual AS (
    SELECT
        e.id,
        count(d.id) AS deposit_count,
        coalesce(sum(d.amount), 0) AS amount,
        count(d.id) FILTER (
            WHERE d.user_id IS DISTINCT FROM e.user_id
               OR d.asset_id IS DISTINCT FROM e.asset_id
        ) AS wrong_owner_or_asset,
        count(d.id) FILTER (
            WHERE d.amount IS DISTINCT FROM t.amount
        ) AS wrong_amount
    FROM mock_deposit_expectations e
    LEFT JOIN mock_transfer_events t
        ON t.id = ANY(e.transfer_event_ids)
    LEFT JOIN deposits d ON d.transfer_event_id = t.id
    GROUP BY e.id
)
SELECT
    e.test_case,
    e.address,
    e.user_id,
    e.transfer_count,
    e.expected_deposit_count,
    a.deposit_count AS actual_deposit_count,
    e.expected_amount / power(10::numeric, asset.decimals) AS expected_usdt,
    a.amount / power(10::numeric, asset.decimals) AS actual_usdt,
    a.deposit_count = e.expected_deposit_count
        AND a.amount = e.expected_amount
        AND a.wrong_owner_or_asset = 0
        AND a.wrong_amount = 0 AS passed
FROM mock_deposit_expectations e
JOIN actual a ON a.id = e.id
JOIN assets asset ON asset.id = e.asset_id
ORDER BY e.id;
