import { Link } from 'react-router-dom';
import { Check, UtensilsCrossed } from 'lucide-react';
import StarRating, { StarRatingReadOnly } from '../../components/StarRating';
import Menu, { type MenuItem } from '../../components/ui/Menu';
import Stepper from '../../components/ui/Stepper';
import { formatDate, servingsLabel } from '../../utils/format';
import type { MealPlanRecipe, PlanStatus } from '../../types';

interface DinnerRowProps {
  dinner: MealPlanRecipe;
  status: PlanStatus;
  onServings: (servings: number) => void;
  onComplete: (completed: boolean) => void;
  onSkip: (skipped: boolean) => void;
  onRemove: () => void;
  onRate: (rating: number) => void;
}

export default function DinnerRow({ dinner, status, onServings, onComplete, onSkip, onRemove, onRate }: DinnerRowProps) {
  const { recipe, completed, skipped } = dinner;
  const minutes = recipe.prep_time_minutes + recipe.cook_time_minutes;
  const open = !completed && !skipped;

  const menu: MenuItem[] = [];
  if (status === 'active') {
    if (completed) menu.push({ label: 'Mark as not cooked', onSelect: () => onComplete(false) });
    else if (skipped) menu.push({ label: 'Undo “not making it”', onSelect: () => onSkip(false) });
    else menu.push({ label: 'Not making it', onSelect: () => onSkip(true) });
  }
  if (status !== 'completed') menu.push({ label: 'Remove from plan', danger: true, onSelect: onRemove });

  return (
    <article className={`dinner-row${skipped ? ' dinner-row-skipped' : ''}`}>
      {recipe.image_url
        ? <img className="dinner-row-img" src={recipe.image_url} alt="" loading="lazy" />
        : <div className="dinner-row-img dinner-row-img-empty" aria-hidden="true"><UtensilsCrossed size={28} /></div>}

      <div className="dinner-row-main">
        <Link to={`/recipe/${recipe.id}`} className="dinner-row-title">{recipe.title}</Link>
        <span className="dinner-row-meta">
          {[recipe.cuisine_type, minutes > 0 && `${minutes} min`].filter(Boolean).join(' · ')}
        </span>
        {completed && (
          <div className="dinner-row-rating">
            {status === 'completed'
              ? <StarRatingReadOnly value={dinner.rating ?? null} />
              : <StarRating value={dinner.rating ?? null} onChange={onRate} />}
          </div>
        )}
      </div>

      <div className="dinner-row-side">
        {completed && (
          <span className="chip chip-success">
            <Check size={14} strokeWidth={2.6} aria-hidden="true" />
            Cooked{dinner.completed_at && ` ${formatDate(dinner.completed_at)}`}
          </span>
        )}
        {skipped && <span className="chip">Not making it</span>}
        {open && status !== 'completed' && (
          <Stepper label="Servings" value={dinner.servings} onChange={onServings} format={servingsLabel} />
        )}
        {open && status === 'active' && (
          <button type="button" className="btn btn-secondary" onClick={() => onComplete(true)}>Cooked it</button>
        )}
        {open && status === 'completed' && <span className="chip">Not cooked</span>}
        <Menu label={`Actions for ${recipe.title}`} items={menu} />
      </div>
    </article>
  );
}
