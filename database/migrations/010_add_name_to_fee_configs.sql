-- Add human-readable name field to fee_configs
ALTER TABLE fee_configs
    ADD COLUMN IF NOT EXISTS name VARCHAR(100) NOT NULL DEFAULT '';