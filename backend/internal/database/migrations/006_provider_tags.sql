-- Add tags column to ollama_providers
ALTER TABLE ollama_providers
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

-- Create index for filtering by tags
CREATE INDEX IF NOT EXISTS idx_ollama_providers_tags ON ollama_providers USING GIN (tags);
