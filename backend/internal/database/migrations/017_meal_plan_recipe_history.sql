-- Track when a recipe was completed in a plan and the user's 1-10 rating.
-- completed_at is NULL until the user toggles "Mark as Done"; cleared when toggled off.
-- rating is NULL until the user picks a star; cleared when the recipe is un-completed.
-- Existing rows backfill as NULL: historical timestamps and scores are unknown.
ALTER TABLE meal_plan_recipes
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS rating       SMALLINT NULL
        CHECK (rating IS NULL OR rating BETWEEN 1 AND 10);

-- Helps GetRecipeHistory and ListRecipeEatCounts skip un-completed rows fast.
CREATE INDEX IF NOT EXISTS idx_meal_plan_recipes_completed
    ON meal_plan_recipes (recipe_id) WHERE completed;
