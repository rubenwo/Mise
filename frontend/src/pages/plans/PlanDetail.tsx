import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  addRecipeToPlan, getPlan, getPlanIngredients, librarySearch,
  randomizePlan, removeRecipeFromPlan, updatePlan, updatePlanRecipe,
} from '../../api/client';
import RecipeCard from '../../components/RecipeCard';
import { revalidateEatCounts } from '../../hooks/useEatCounts';
import { errorMessage, useFetch } from '../../hooks/useFetch';
import { useInventory } from '../../hooks/useInventory';
import { planStatusLabel } from '../../utils/format';
import type { MealPlan, Recipe } from '../../types';
import ActivePlanView from './ActivePlanView';
import BrowseRecipeCard from './BrowseRecipeCard';
import CompletedPlanView from './CompletedPlanView';
import IngredientSummary from './IngredientSummary';

export default function PlanDetail({ planId }: { planId: number }) {
  const planQuery = useFetch(`plan:${planId}`, signal => getPlan(planId, signal));
  const plan = planQuery.data;
  const setPlan = planQuery.setData;
  // The shopping list is only shown while drafting; it can trigger an LLM
  // dedup pass on the backend, so skip it for active/completed plans.
  const ingredientsQuery = useFetch(
    plan?.status === 'draft' ? `plan-ingredients:${planId}` : null,
    signal => getPlanIngredients(planId, signal),
  );
  const ingredients = ingredientsQuery.data ?? [];

  const inventory = useInventory();
  const [servingsInput, setServingsInput] = useState<Record<number, number>>({});
  const [adding, setAdding] = useState<Record<number, boolean>>({});
  const [recipeFilter, setRecipeFilter] = useState('');
  const [debouncedFilter, setDebouncedFilter] = useState('');
  const [randomDays, setRandomDays] = useState([4]);
  const [randomizing, setRandomizing] = useState(false);
  const [removedIds, setRemovedIds] = useState<Set<number>>(new Set());

  // Debounced backend search for the recipe browser (immediate when cleared).
  useEffect(() => {
    const trimmed = recipeFilter.trim();
    const timer = setTimeout(() => setDebouncedFilter(trimmed), trimmed ? 300 : 0);
    return () => clearTimeout(timer);
  }, [recipeFilter]);

  const browse = useFetch(
    `plan-browse:${debouncedFilter}`,
    signal => librarySearch({ keywords: debouncedFilter, limit: 100 }, signal),
  );
  const browseRecipes = browse.data?.recipes ?? [];

  /** Runs a plan mutation, applies the returned plan, and reports failures. */
  const mutate = async (
    action: () => Promise<MealPlan>,
    { ingredients: refreshIngredients = false, eatCounts = false } = {},
  ) => {
    try {
      setPlan(await action());
      if (refreshIngredients) ingredientsQuery.reload();
      if (eatCounts) revalidateEatCounts();
    } catch (err) {
      alert('Something went wrong: ' + errorMessage(err));
    }
  };

  const defaultServings = (recipe: Recipe) => servingsInput[recipe.id] || recipe.servings || 4;

  const handleAdd = async (recipe: Recipe) => {
    setAdding(prev => ({ ...prev, [recipe.id]: true }));
    await mutate(() => addRecipeToPlan(planId, recipe.id, defaultServings(recipe)), { ingredients: true });
    setAdding(prev => ({ ...prev, [recipe.id]: false }));
  };

  const handleRemove = async (recipeId: number) => {
    setRemovedIds(prev => new Set([...prev, recipeId]));
    await mutate(() => removeRecipeFromPlan(planId, recipeId), { ingredients: true });
  };

  const handleServingsChange = (recipeId: number, servings: number) =>
    mutate(() => updatePlanRecipe(planId, recipeId, { servings }), { ingredients: true });

  const handleStartPeriod = () => mutate(() => updatePlan(planId, { status: 'active' }));
  const handleEndPeriod = () => mutate(() => updatePlan(planId, { status: 'completed' }));

  const handleComplete = (recipeId: number, completed: boolean) =>
    mutate(() => updatePlanRecipe(planId, recipeId, { completed }), { eatCounts: true });

  const handleSkip = (recipeId: number, skipped: boolean) =>
    mutate(() => updatePlanRecipe(planId, recipeId, { skipped }), { eatCounts: true });

  const handleRemoveActive = (recipeId: number) => {
    if (!confirm('Remove this recipe from the plan? It will no longer appear in your history.')) return;
    mutate(() => removeRecipeFromPlan(planId, recipeId));
  };

  // rating: 1-10 sets, 0 clears; only offered once a recipe is completed.
  const handleRate = (recipeId: number, rating: number) =>
    mutate(() => updatePlanRecipe(planId, recipeId, { rating }), { eatCounts: true });

  const handleRandomize = async () => {
    setRandomizing(true);
    await mutate(() => randomizePlan(planId, randomDays, [...removedIds]), { ingredients: true });
    setRemovedIds(new Set());
    setRandomizing(false);
  };

  if (planQuery.loading && !plan) return <p className="empty-state">Loading...</p>;
  if (planQuery.error) return <div className="error-message">{planQuery.error}</div>;
  if (!plan) return <p className="empty-state">Plan not found.</p>;

  const planRecipes = plan.recipes ?? [];
  const planRecipeIds = new Set(planRecipes.map(r => r.recipe_id));
  const availableRecipes = browseRecipes.filter(r => !planRecipeIds.has(r.id));
  const loadingIngredients = ingredientsQuery.loading && !ingredientsQuery.data;
  const isActive = plan.status === 'active';
  const isCompleted = plan.status === 'completed';
  const isDraft = plan.status === 'draft';
  const completedCount = planRecipes.filter(r => r.completed).length;
  const resolvedCount = planRecipes.filter(r => r.completed || r.skipped).length;

  return (
    <div className="plan-detail">
      <div className="plan-detail-header">
        <Link to="/plans" className="btn btn-secondary">Back</Link>
        <h2>{plan.name}</h2>
        <span className={`plan-status plan-status-${plan.status}`}>
          {planStatusLabel(plan.status)}
        </span>
      </div>

      {isActive && (
        <ActivePlanView
          plan={plan}
          onComplete={handleComplete}
          onSkip={handleSkip}
          onRemove={handleRemoveActive}
          onRate={handleRate}
          onEnd={handleEndPeriod}
          completedCount={completedCount}
          resolvedCount={resolvedCount}
        />
      )}

      {isCompleted && <CompletedPlanView plan={plan} />}

      {isDraft && loadingIngredients && (
        <div className="plan-section plan-loading">
          <div className="plan-loading-spinner" />
          <p>Normalizing ingredients...</p>
        </div>
      )}

      {isDraft && !loadingIngredients && (
        <>
          {/* Randomize bar */}
          <div className="plan-section plan-randomize-bar">
            <h3>Randomize</h3>
            <p className="plan-randomize-hint">Set the total number of days and servings per day. Randomize fills empty slots — recipes you&apos;ve already kept stay in the plan.</p>
            <div className="plan-randomize-days">
              {randomDays.map((s, i) => (
                <div key={i} className="plan-randomize-day">
                  <label>Day {i + 1}</label>
                  <input
                    type="number"
                    min="1"
                    max="20"
                    value={s}
                    onChange={e => setRandomDays(prev => prev.map((v, j) => j === i ? (parseInt(e.target.value) || 1) : v))}
                  />
                  {randomDays.length > 1 && (
                    <button className="btn btn-danger btn-sm" onClick={() => setRandomDays(prev => prev.filter((_, j) => j !== i))}>
                      &times;
                    </button>
                  )}
                </div>
              ))}
            </div>
            <div className="plan-randomize-actions">
              <button className="btn btn-secondary" onClick={() => setRandomDays(prev => [...prev, prev[prev.length - 1] || 4])}>
                + Add Day
              </button>
              <button className="btn btn-primary" onClick={handleRandomize} disabled={randomizing}>
                {randomizing ? 'Randomizing...' : (() => {
                  const have = planRecipes.length;
                  const need = randomDays.length - have;
                  if (need <= 0) return `${randomDays.length} days (plan full)`;
                  if (have === 0) return `Randomize ${randomDays.length} days`;
                  return `Add ${need} more recipe${need !== 1 ? 's' : ''}`;
                })()}
              </button>
            </div>
          </div>

          {/* Selected recipes */}
          <div className="plan-section">
            <h3>Selected Recipes ({planRecipes.length})</h3>
            {planRecipes.length === 0 ? (
              <p className="empty-state">No recipes selected yet. Add some below.</p>
            ) : (
              <>
                <div className="plan-recipes">
                  {planRecipes.map(mpr => (
                    <div key={mpr.recipe_id} className="plan-recipe-item">
                      <RecipeCard recipe={mpr.recipe} />
                      <div className="plan-recipe-controls">
                        <label>
                          Servings:
                          <input
                            type="number"
                            min="1"
                            max="20"
                            value={mpr.servings}
                            onChange={e => handleServingsChange(mpr.recipe_id, parseInt(e.target.value) || 1)}
                          />
                        </label>
                        <button className="btn btn-danger" onClick={() => handleRemove(mpr.recipe_id)}>Remove</button>
                      </div>
                    </div>
                  ))}
                </div>
                {ingredients.length > 0 && <IngredientSummary ingredients={ingredients} planId={planId} />}
                <button className="btn btn-primary plan-start-btn" onClick={handleStartPeriod}>
                  Start Period
                </button>
              </>
            )}
          </div>

          {/* Recipe browser */}
          <div className="plan-section">
            <h3>Add Recipes</h3>
            <div className="search-bar" style={{ marginBottom: 16 }}>
              <input
                type="text"
                placeholder="Filter recipes..."
                value={recipeFilter}
                onChange={e => setRecipeFilter(e.target.value)}
              />
            </div>
            <div className="plan-recipe-browser">
              {availableRecipes.map(recipe => (
                <BrowseRecipeCard
                  key={recipe.id}
                  recipe={recipe}
                  servings={defaultServings(recipe)}
                  onServingsChange={v => setServingsInput(prev => ({ ...prev, [recipe.id]: v }))}
                  onAdd={() => handleAdd(recipe)}
                  adding={!!adding[recipe.id]}
                  inventory={inventory}
                />
              ))}
              {availableRecipes.length === 0 && (
                <p className="empty-state">{recipeFilter ? 'No recipes match your filter.' : 'Type to search your recipe library.'}</p>
              )}
            </div>
          </div>
        </>
      )}

    </div>
  );
}
