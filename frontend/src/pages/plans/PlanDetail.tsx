import { useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, Plus, Shuffle } from 'lucide-react';
import {
  addRecipeToPlan, getPlan, getPlanIngredients,
  randomizePlan, removeRecipeFromPlan, updatePlan, updatePlanRecipe,
} from '../../api/client';
import Stepper from '../../components/ui/Stepper';
import { revalidateEatCounts } from '../../hooks/useEatCounts';
import { errorMessage, useFetch } from '../../hooks/useFetch';
import { useInventory } from '../../hooks/useInventory';
import { planStatusLabel, servingsLabel } from '../../utils/format';
import type { MealPlan } from '../../types';
import AddRecipesPanel from './AddRecipesPanel';
import DinnerRow from './DinnerRow';
import GroceryPanel from './GroceryPanel';

type Panel = 'none' | 'add' | 'randomize';

export default function PlanDetail({ planId }: { planId: number }) {
  const planQuery = useFetch(`plan:${planId}`, signal => getPlan(planId, signal));
  const plan = planQuery.data;
  const setPlan = planQuery.setData;
  // Finished plans don't need a shopping list (building one can cost an LLM pass).
  const ingredientsQuery = useFetch(
    plan && plan.status !== 'completed' ? `plan-ingredients:${planId}` : null,
    signal => getPlanIngredients(planId, signal),
  );
  const inventory = useInventory();
  const [panel, setPanel] = useState<Panel>('none');
  const [randomDays, setRandomDays] = useState([2]);
  const [randomizing, setRandomizing] = useState(false);
  const [removedIds, setRemovedIds] = useState<Set<number>>(new Set());

  /** Runs a plan mutation, applies the returned plan, and reports failures. */
  const mutate = async (
    action: () => Promise<MealPlan>,
    { ingredients = false, eatCounts = false } = {},
  ) => {
    try {
      setPlan(await action());
      if (ingredients) ingredientsQuery.reload();
      if (eatCounts) revalidateEatCounts();
    } catch (err) {
      alert('Something went wrong: ' + errorMessage(err));
    }
  };

  if (planQuery.loading && !plan) return <p className="empty-state">Loading...</p>;
  if (planQuery.error) return <div className="error-message">{planQuery.error}</div>;
  if (!plan) return <p className="empty-state">Plan not found.</p>;

  const dinners = plan.recipes ?? [];
  const cooked = dinners.filter(d => d.completed).length;
  const rated = dinners.filter(d => d.rating != null);
  const avgRating = rated.length ? rated.reduce((s, d) => s + (d.rating ?? 0), 0) / rated.length : null;
  const editable = plan.status !== 'completed';

  const remove = (recipeId: number) => {
    if (plan.status === 'active' && !confirm('Remove this dinner from the plan? Its cook history goes with it.')) return;
    setRemovedIds(prev => new Set([...prev, recipeId]));
    mutate(() => removeRecipeFromPlan(planId, recipeId), { ingredients: true });
  };

  const randomize = async () => {
    setRandomizing(true);
    await mutate(() => randomizePlan(planId, randomDays, [...removedIds]), { ingredients: true });
    setRemovedIds(new Set());
    setRandomizing(false);
  };

  const togglePanel = (p: Panel) => setPanel(cur => (cur === p ? 'none' : p));

  return (
    <div className="plan-page">
      <header className="plan-page-header">
        <div className="plan-page-heading">
          <Link to="/plans" className="back-link"><ArrowLeft size={16} aria-hidden="true" />All plans</Link>
          <div className="plan-page-title">
            <h1>{plan.name}</h1>
            <span className={`plan-status plan-status-${plan.status}`}>{planStatusLabel(plan.status)}</span>
          </div>
          <p className="plan-page-summary">
            {dinners.length} {dinners.length === 1 ? 'dinner' : 'dinners'} · {cooked} cooked
            {avgRating != null && ` · average ${avgRating.toFixed(1)}/10`}
          </p>
        </div>
        <div className="plan-page-actions">
          {plan.status === 'draft' && (
            <button type="button" className="btn btn-secondary" aria-pressed={panel === 'randomize'} onClick={() => togglePanel('randomize')}>
              <Shuffle size={16} aria-hidden="true" />Fill with suggestions
            </button>
          )}
          {editable && (
            <button type="button" className="btn btn-secondary" aria-pressed={panel === 'add'} onClick={() => togglePanel('add')}>
              <Plus size={16} aria-hidden="true" />Add from library
            </button>
          )}
          {plan.status === 'draft' && dinners.length > 0 && (
            <button type="button" className="btn btn-primary" onClick={() => mutate(() => updatePlan(planId, { status: 'active' }))}>
              Start plan
            </button>
          )}
          {plan.status === 'active' && (
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => confirm('Finish this plan? It moves to your history.') && mutate(() => updatePlan(planId, { status: 'completed' }))}
            >
              Finish plan
            </button>
          )}
        </div>
      </header>

      {panel === 'randomize' && (
        <section className="plan-panel" aria-labelledby="randomize-title">
          <h2 id="randomize-title">Fill with suggestions</h2>
          <p className="plan-panel-hint">
            Picks dinners from your library for the empty slots. Dinners already in the plan stay.
          </p>
          <div className="randomize-slots">
            {randomDays.map((n, i) => (
              <div key={i} className="randomize-slot">
                <span>Dinner {i + 1}</span>
                <Stepper label="Servings" value={n} format={servingsLabel}
                  onChange={v => setRandomDays(prev => prev.map((x, j) => (j === i ? v : x)))} />
                {randomDays.length > 1 && (
                  <button type="button" className="btn-link" onClick={() => setRandomDays(prev => prev.filter((_, j) => j !== i))}>Remove</button>
                )}
              </div>
            ))}
          </div>
          <div className="plan-panel-actions">
            <button type="button" className="btn btn-secondary" onClick={() => setRandomDays(prev => [...prev, prev[prev.length - 1] || 2])}>
              Add a dinner
            </button>
            <button type="button" className="btn btn-primary" onClick={randomize} disabled={randomizing || randomDays.length <= dinners.length}>
              {randomizing ? 'Picking…' : randomDays.length <= dinners.length
                ? 'Plan is full'
                : `Pick ${randomDays.length - dinners.length} ${randomDays.length - dinners.length === 1 ? 'dinner' : 'dinners'}`}
            </button>
          </div>
        </section>
      )}

      {panel === 'add' && (
        <AddRecipesPanel
          planRecipeIds={new Set(dinners.map(d => d.recipe_id))}
          inventory={inventory}
          onAdd={(recipe, servings) => mutate(() => addRecipeToPlan(planId, recipe.id, servings), { ingredients: true })}
        />
      )}

      <div className="plan-layout">
        <section className="dinner-list" aria-label="Dinners">
          {dinners.length === 0 ? (
            <div className="empty-panel">
              <p>No dinners in this plan yet.</p>
              <button type="button" className="btn btn-primary" onClick={() => setPanel('add')}>Add from library</button>
            </div>
          ) : (
            dinners.map(d => (
              <DinnerRow
                key={d.recipe_id}
                dinner={d}
                status={plan.status}
                onServings={servings => mutate(() => updatePlanRecipe(planId, d.recipe_id, { servings }), { ingredients: true })}
                onComplete={completed => mutate(() => updatePlanRecipe(planId, d.recipe_id, { completed }), { eatCounts: true })}
                onSkip={skipped => mutate(() => updatePlanRecipe(planId, d.recipe_id, { skipped }), { eatCounts: true })}
                onRate={rating => mutate(() => updatePlanRecipe(planId, d.recipe_id, { rating }), { eatCounts: true })}
                onRemove={() => remove(d.recipe_id)}
              />
            ))
          )}
        </section>

        {editable && (
          <GroceryPanel
            planId={planId}
            ingredients={ingredientsQuery.data ?? []}
            loading={ingredientsQuery.loading && !ingredientsQuery.data}
            inventory={inventory}
          />
        )}
      </div>
    </div>
  );
}
