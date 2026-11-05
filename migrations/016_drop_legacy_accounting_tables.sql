-- Run after 015, which has copied and validated the old accounting data.
-- Run the entire file in one transaction (psql --single-transaction).
-- RESTRICT prevents deletion if an unexpected dependency still uses either table.
DROP TABLE ledger_legacy, balances_legacy RESTRICT;
