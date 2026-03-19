-- Stores background-generated recipes awaiting user review.
-- Approved recipes are moved to the recipes table; rejected ones are deleted.
-- Rows older than 7 days are automatically discarded by the cleanup job.
CREATE TABLE IF NOT EXISTS pending_recipes (
    id               SERIAL PRIMARY KEY,
    title            TEXT NOT NULL,
    description      TEXT,
    cuisine_type     TEXT,
    prep_time_minutes INT DEFAULT 0,
    cook_time_minutes INT DEFAULT 0,
    servings         INT DEFAULT 4,
    difficulty       TEXT,
    ingredients      JSONB NOT NULL DEFAULT '[]',
    instructions     JSONB NOT NULL DEFAULT '[]',
    dietary_restrictions TEXT[] DEFAULT '{}',
    tags             TEXT[] DEFAULT '{}',
    generated_by_model TEXT,
    image_url        TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pending_recipes_created_at ON pending_recipes (created_at);
