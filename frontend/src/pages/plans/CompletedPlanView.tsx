import { Link } from 'react-router-dom';
import { StarRatingReadOnly } from '../../components/StarRating';
import { formatDate } from '../../utils/format';
import type { MealPlan } from '../../types';

// Read-only view shown after a plan is ended. Lists every recipe with its
// done/skipped state, the date it was cooked, and its rating.
export default function CompletedPlanView({ plan }: { plan: MealPlan }) {
  const recipes = plan.recipes || [];
  const doneCount = recipes.filter(r => r.completed).length;
  const ratedCount = recipes.filter(r => r.rating != null).length;
  const avgRating = ratedCount > 0
    ? recipes.reduce((sum, r) => sum + (r.rating || 0), 0) / ratedCount
    : null;

  return (
    <div className="completed-plan">
      <div className="completed-plan-summary">
        <span><strong>{doneCount}</strong> of {recipes.length} cooked</span>
        {avgRating != null && (
          <span> · avg rating <strong>{avgRating.toFixed(1)}/10</strong> ({ratedCount} rated)</span>
        )}
      </div>

      <ul className="completed-plan-list">
        {recipes.map(mpr => (
          <li key={mpr.recipe_id} className={`completed-plan-item${mpr.completed ? '' : ' completed-plan-item-skipped'}`}>
            <div className="completed-plan-item-main">
              <Link to={`/recipe/${mpr.recipe_id}`} className="completed-plan-title">
                {mpr.recipe?.title || `Recipe ${mpr.recipe_id}`}
              </Link>
              {mpr.recipe?.cuisine_type && (
                <span className="cuisine-badge">{mpr.recipe.cuisine_type}</span>
              )}
            </div>
            <div className="completed-plan-item-meta">
              {mpr.completed
                ? <span className="completed-plan-status completed-plan-status-done">✓ cooked{mpr.completed_at && ` · ${formatDate(mpr.completed_at)}`}</span>
                : mpr.skipped
                  ? <span className="completed-plan-status completed-plan-status-skipped">– skipped</span>
                  : <span className="completed-plan-status completed-plan-status-skipped">✗ not cooked</span>}
              {mpr.completed && (
                <div className="completed-plan-rating">
                  <StarRatingReadOnly value={mpr.rating ?? null} />
                </div>
              )}
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
