CREATE TABLE IF NOT EXISTS recipes (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    cuisine_type TEXT NOT NULL DEFAULT '',
    prep_time_minutes INTEGER NOT NULL DEFAULT 0,
    cook_time_minutes INTEGER NOT NULL DEFAULT 0,
    servings INTEGER NOT NULL DEFAULT 4,
    difficulty TEXT NOT NULL DEFAULT 'medium',
    ingredients JSONB NOT NULL DEFAULT '[]'::jsonb,
    instructions JSONB NOT NULL DEFAULT '[]'::jsonb,
    dietary_restrictions TEXT[] NOT NULL DEFAULT '{}',
    tags TEXT[] NOT NULL DEFAULT '{}',
    generated_by_model TEXT NOT NULL DEFAULT '',
    generation_prompt TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recipes_cuisine_type ON recipes (cuisine_type);
CREATE INDEX IF NOT EXISTS idx_recipes_difficulty ON recipes (difficulty);
CREATE INDEX IF NOT EXISTS idx_recipes_dietary_restrictions ON recipes USING GIN (dietary_restrictions);
CREATE INDEX IF NOT EXISTS idx_recipes_tags ON recipes USING GIN (tags);
CREATE INDEX IF NOT EXISTS idx_recipes_fulltext ON recipes USING GIN (
    to_tsvector('english', coalesce(title, '') || ' ' || coalesce(description, ''))
);
