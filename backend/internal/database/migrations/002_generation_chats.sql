CREATE TABLE IF NOT EXISTS generation_chats (
    id SERIAL PRIMARY KEY,
    prompt TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    messages JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
