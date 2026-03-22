-- Add provider_type to ollama_providers to support OpenAI-compatible endpoints
-- in addition to native Ollama. Existing rows default to 'ollama'.
ALTER TABLE ollama_providers
    ADD COLUMN IF NOT EXISTS provider_type TEXT NOT NULL DEFAULT 'ollama';
