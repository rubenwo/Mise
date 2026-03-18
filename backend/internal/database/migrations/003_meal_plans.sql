CREATE TABLE IF NOT EXISTS meal_plans (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS meal_plan_recipes (
    id SERIAL PRIMARY KEY,
    meal_plan_id INT NOT NULL REFERENCES meal_plans(id) ON DELETE CASCADE,
    recipe_id INT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    servings INT NOT NULL DEFAULT 4,
    sort_order INT NOT NULL DEFAULT 0,
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE(meal_plan_id, recipe_id)
);
