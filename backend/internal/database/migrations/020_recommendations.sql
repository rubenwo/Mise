-- Text embeddings per recipe for the recommender. text_hash identifies the
-- embedded text (and model), so edits re-embed only the recipes that changed.
CREATE TABLE IF NOT EXISTS recipe_embeddings (
    recipe_id INTEGER PRIMARY KEY REFERENCES recipes(id) ON DELETE CASCADE,
    model TEXT NOT NULL,
    text_hash TEXT NOT NULL,
    embedding REAL[] NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One row per recipe per day it was recommended. Recipes shown repeatedly
-- without ever being planned fade out of the picks.
CREATE TABLE IF NOT EXISTS recommendation_impressions (
    recipe_id INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    shown_on DATE NOT NULL DEFAULT CURRENT_DATE,
    PRIMARY KEY (recipe_id, shown_on)
);
