-- Add health tracking columns to ollama_providers
ALTER TABLE ollama_providers
    ADD COLUMN IF NOT EXISTS health_status TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS last_health_check TIMESTAMPTZ DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS last_error TEXT DEFAULT NULL;

-- Create index for filtering by health status
CREATE INDEX IF NOT EXISTS idx_ollama_providers_health ON ollama_providers (health_status);
