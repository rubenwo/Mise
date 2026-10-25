import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { Plus, Shuffle } from 'lucide-react';
import {
  getPlan, getRecommendations, getSettings, listCuisines, listPendingRecipes, listPlans, recordImpressions, updatePlanRecipe,
} from '../api/client';
import AddToPlanMenu from '../components/AddToPlanMenu';
import { revalidateEatCounts } from '../hooks/useEatCounts';
import { errorMessage, useFetch } from '../hooks/useFetch';
import type { CuisineMeta, MealPlan, RecommendedRecipe } from '../types';

// Default number of home picks; Settings → General → Home picks overrides it.
const DEFAULT_PICKS = 3;

function PickCard({ pick }: { pick: RecommendedRecipe }) {
  const { recipe } = pick;
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLButtonElement>(null);
  const minutes = recipe.prep_time_minutes + recipe.cook_time_minutes;
  return (
    <article className="pick-card">
      <Link to={`/recipe/${recipe.id}`} className="pick-card-link">
        {recipe.image_url
          ? <img src={recipe.image_url} alt="" className="pick-card-img" />
          : <div className="pick-card-img lib-card-img-empty" aria-hidden="true" />}
        <div className="pick-card-body">
          <span className="lib-card-eyebrow">{[recipe.cuisine_type, minutes > 0 && `${minutes} min`].filter(Boolean).join(' · ')}</span>
          <span className="pick-card-title">{recipe.title}</span>
          {recipe.description && <span className="pick-card-desc">{recipe.description}</span>}
          <span className="lib-card-reason">{pick.reason}</span>
        </div>
      </Link>
      <div className="pick-card-actions">
        <button ref={ref} type="button" className="btn btn-primary" onClick={() => setOpen(o => !o)}>
          <Plus size={16} aria-hidden="true" />Add to plan
        </button>
        {open && <AddToPlanMenu recipeId={recipe.id} anchorRef={ref} onClose={() => setOpen(false)} />}
      </div>
    </article>
  );
}

/** The plan you're shopping for: the active one, else the newest draft. */
function currentPlan(plans: MealPlan[] | undefined) {
  return plans?.find(p => p.status === 'active') ?? plans?.find(p => p.status === 'draft');
}

function PlanCard({ planId }: { planId: number }) {
  const plan = useFetch(`plan:${planId}`, signal => getPlan(planId, signal));
  const dinners = plan.data?.recipes ?? [];
  const open = plan.data?.status === 'active' ? dinners.filter(d => !d.completed && !d.skipped) : [];

  const markCooked = async (recipeId: number) => {
    try {
      plan.setData(await updatePlanRecipe(planId, recipeId, { completed: true }));
      revalidateEatCounts();
    } catch (err) {
      alert('Could not save: ' + errorMessage(err));
    }
  };

  if (!plan.data) return null;
  const cooked = dinners.filter(d => d.completed).length;
  return (
    <>
      <Link to={`/plans/${planId}`} className="home-card home-plan">
        <span className="home-card-kicker">Current plan</span>
        <span className="home-plan-name">{plan.data.name}</span>
        <span className="home-plan-thumbs" aria-hidden="true">
          {dinners.slice(0, 6).map(d => d.recipe.image_url
            ? <img key={d.recipe_id} src={d.recipe.image_url} alt="" />
            : <span key={d.recipe_id} className="home-plan-thumb-empty" />)}
        </span>
        <span className="home-card-text">{dinners.length} dinners · {cooked} cooked</span>
        <span className="home-card-link">Open plan →</span>
      </Link>
      {open.length > 0 && (
        <section className="home-card" aria-labelledby="home-cooked">
          <h2 id="home-cooked">Cooked something from the plan?</h2>
          <ul className="home-mark-list">
            {open.map(d => (
              <li key={d.recipe_id}>
                {d.recipe.image_url ? <img src={d.recipe.image_url} alt="" /> : <span className="home-plan-thumb-empty" />}
                <Link to={`/recipe/${d.recipe_id}`}>{d.recipe.title}</Link>
                <button type="button" className="btn btn-secondary btn-sm" onClick={() => markCooked(d.recipe_id)}>Cooked it</button>
              </li>
            ))}
          </ul>
          <p className="home-card-text">Rate it on the plan page once you’ve eaten.</p>
        </section>
      )}
    </>
  );
}

/** A few cuisines to browse, rotating daily. */
function cuisinesOfTheDay(cuisines: CuisineMeta[]): CuisineMeta[] {
  const day = Math.floor(Date.now() / 864e5);
  const withPhotos = cuisines.filter(c => c.preview_images?.length);
  return Array.from({ length: Math.min(5, withPhotos.length) }, (_, i) => withPhotos[(day * 7 + i * 11) % withPhotos.length])
    .filter((c, i, arr) => arr.indexOf(c) === i);
}

export default function HomePage() {
  const [round, setRound] = useState(0);
  const settings = useFetch('settings', getSettings);
  const configured = parseInt(settings.data?.find(s => s.key === 'suggestion_count')?.value ?? '', 10);
  const count = Math.min(10, Math.max(1, configured || DEFAULT_PICKS));
  const picks = useFetch(
    settings.data || settings.error ? `home-picks:${count}:${round}` : null,
    signal => getRecommendations({ limit: count, offset: round * count }, signal),
  );
  const plans = useFetch('plans', listPlans);
  const pending = useFetch('pending', listPendingRecipes);
  const cuisines = useFetch('cuisines', listCuisines);
  const plan = currentPlan(plans.data);
  const pendingList = pending.data ?? [];

  const shownIds = (picks.data ?? []).map(p => p.recipe.id).join(',');
  useEffect(() => {
    if (shownIds) recordImpressions(shownIds.split(',').map(Number)).catch(() => {});
  }, [shownIds]);

  return (
    <div className="home">
      <div className="home-header">
        <div>
          <p className="home-kicker">Fresh picks · refreshed daily</p>
          <h1>Something new to cook</h1>
        </div>
        <div className="home-header-actions">
          <button type="button" className="btn btn-secondary" onClick={() => setRound(r => (r + 1) % 8)}>
            <Shuffle size={16} aria-hidden="true" />Other ideas
          </button>
          <Link to="/library" className="btn btn-secondary">Browse library</Link>
        </div>
      </div>

      <div className="home-layout">
        <section className="home-picks" aria-label="Fresh picks">
          {picks.error && <div className="error-message">{picks.error}</div>}
          {(picks.data ?? []).map(p => <PickCard key={p.recipe.id} pick={p} />)}
          {!picks.data && !picks.error && Array.from({ length: count }, (_, i) => <div key={i} className="pick-card pick-card-skeleton" />)}
        </section>

        <div className="home-side">
          {plan && <PlanCard planId={plan.id} />}
          {!plan && plans.data && (
            <Link to="/plans" className="home-card home-plan">
              <span className="home-card-kicker">No plan yet</span>
              <span className="home-card-text">Start a plan for your next grocery run.</span>
              <span className="home-card-link">Go to plans →</span>
            </Link>
          )}
          {pendingList.length > 0 && (
            <Link to="/pending" className="home-card">
              <h2>{pendingList.length} new {pendingList.length === 1 ? 'recipe' : 'recipes'} to review</h2>
              <span className="home-review-thumbs" aria-hidden="true">
                {pendingList.slice(0, 3).map(r => r.image_url
                  ? <img key={r.id} src={r.image_url} alt="" />
                  : <span key={r.id} className="home-plan-thumb-empty" />)}
              </span>
              <span className="home-card-link">Review recipes →</span>
            </Link>
          )}
        </div>
      </div>

      {cuisines.data && (
        <section className="home-cuisines" aria-labelledby="home-cuisines">
          <div className="home-section-head">
            <h2 id="home-cuisines">Cuisines to explore</h2>
            <Link to="/library">All cuisines</Link>
          </div>
          <div className="home-cuisine-grid">
            {cuisinesOfTheDay(cuisines.data).map(c => (
              <Link key={c.cuisine_type} to={`/library?cuisine=${encodeURIComponent(c.cuisine_type)}`} className="home-cuisine">
                <img src={c.preview_images[0]} alt="" />
                <span>
                  <strong>{c.cuisine_type}</strong>
                  {c.count} recipes
                </span>
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
