-- Cache normalized ingredient list on meal_plans to avoid re-running LLM normalization on every view.
-- Invalidated (set to NULL) whenever recipes in the plan are added, removed, or have servings changed.
ALTER TABLE meal_plans ADD COLUMN IF NOT EXISTS normalized_ingredients JSONB;
