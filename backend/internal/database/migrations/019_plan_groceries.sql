-- Grocery list state that should survive reloads and sync across devices.
-- Checks are keyed by the aggregated ingredient name; a stale check (for an
-- ingredient no longer in the plan) is harmless and simply never shown.
CREATE TABLE IF NOT EXISTS plan_grocery_checks (
    meal_plan_id INTEGER NOT NULL REFERENCES meal_plans(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    PRIMARY KEY (meal_plan_id, name)
);

-- Last Albert Heijn product search per plan, so the plan page can show what
-- wasn't found without searching again.
CREATE TABLE IF NOT EXISTS plan_ah_searches (
    meal_plan_id INTEGER PRIMARY KEY REFERENCES meal_plans(id) ON DELETE CASCADE,
    result JSONB NOT NULL,
    searched_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
