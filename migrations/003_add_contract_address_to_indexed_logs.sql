ALTER TABLE indexed_logs
ADD COLUMN IF NOT EXISTS contract_address TEXT;