import { useState } from 'react';
import CookingChat from '../../components/CookingChat';
import RecipeCard from '../../components/RecipeCard';
import StarRating from '../../components/StarRating';
import type { MealPlan } from '../../types';

interface ActivePlanViewProps {
  plan: MealPlan;
  onComplete: (recipeId: number, completed: boolean) => void;
  onSkip: (recipeId: number, skipped: boolean) => void;
  onRemove: (recipeId: number) => void;
  onRate: (recipeId: number, rating: number) => void;
  onEnd: () => void;
  completedCount: number;
  resolvedCount: number;
}

export default function ActivePlanView({ plan, onComplete, onSkip, onRemove, onRate, onEnd, completedCount, resolvedCount }: ActivePlanViewProps) {
  const [currentIndex, setCurrentIndex] = useState(0);
  const recipes = plan.recipes || [];
  const total = recipes.length;

  if (total === 0) return <p className="empty-state">No recipes in this plan.</p>;

  const safeIndex = Math.min(currentIndex, recipes.length - 1);
  const current = recipes[safeIndex];

  return (
    <div className="active-plan">
      <div className="active-plan-progress">
        <span>{completedCount} cooked · {resolvedCount} of {total} resolved</span>
        <div className="progress-bar">
          <div className="progress-fill" style={{ width: `${(resolvedCount / total) * 100}%` }} />
        </div>
      </div>

      <div className="active-plan-nav">
        {recipes.map((r, i) => (
          <button
            key={r.recipe_id}
            className={`active-plan-nav-tab${i === safeIndex ? ' active-plan-nav-tab-active' : ''}${r.completed ? ' active-plan-nav-tab-done' : ''}${r.skipped ? ' active-plan-nav-tab-skipped' : ''}`}
            onClick={() => setCurrentIndex(i)}
          >
            {r.recipe?.title || `Recipe ${i + 1}`}
          </button>
        ))}
      </div>

      <div key={current.recipe_id} className={`active-plan-card ${current.completed ? 'active-plan-card-done' : ''} ${current.skipped ? 'active-plan-card-skipped' : ''}`}>
        <RecipeCard recipe={current.recipe} showIngredients showInstructions />
        <div className="active-plan-servings">
          Adjusted servings: <strong>{current.servings}</strong>
          {current.servings !== current.recipe.servings && (
            <span className="servings-note"> (original: {current.recipe.servings})</span>
          )}
        </div>
        {current.skipped ? (
          <div className="active-plan-skip-notice">
            <span>Marked as not made.</span>
            <button className="btn btn-secondary" onClick={() => onSkip(current.recipe_id, false)}>
              Undo skip
            </button>
          </div>
        ) : (
          <div className="active-plan-actions">
            <button
              className={`btn ${current.completed ? 'btn-secondary' : 'btn-primary'} active-plan-complete-btn`}
              onClick={() => onComplete(current.recipe_id, !current.completed)}
            >
              {current.completed ? 'Mark as Not Done' : 'Mark as Done'}
            </button>
            {!current.completed && (
              <button
                className="btn btn-secondary"
                onClick={() => onSkip(current.recipe_id, true)}
                title="I'm not going to make this one"
              >
                Won't make
              </button>
            )}
            <button
              className="btn btn-danger"
              onClick={() => onRemove(current.recipe_id)}
              title="Remove from plan entirely"
            >
              Remove
            </button>
          </div>
        )}
        {current.completed && (
          <div className="active-plan-rating">
            <StarRating
              value={current.rating ?? null}
              onChange={(rating) => onRate(current.recipe_id, rating)}
            />
          </div>
        )}
        <CookingChat key={current.recipe_id} recipeId={current.recipe_id} />
      </div>

      {resolvedCount === total && (
        <button className="btn btn-primary plan-start-btn" onClick={onEnd}>
          End Period
        </button>
      )}
    </div>
  );
}
