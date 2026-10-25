import { useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { Plus, Star, UtensilsCrossed } from 'lucide-react';
import AddToPlanMenu from './AddToPlanMenu';
import type { Recipe, RecipeEatStats } from '../types';

interface LibraryCardProps {
  recipe: Recipe;
  /** Discovery views describe the dish; Favourites shows how it went. */
  detail: 'description' | 'rating';
  stats?: RecipeEatStats;
  /** Why a recommendation was picked. */
  reason?: string;
}

/** Adds a recipe to a plan from a card without opening the recipe. */
function PlanButton({ recipeId, title }: { recipeId: number; title: string }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={ref} type="button" className="lib-card-plan" aria-label={`Add ${title} to a plan`} onClick={() => setOpen(o => !o)}>
        <Plus size={18} strokeWidth={2.2} aria-hidden="true" />
      </button>
      {open && <AddToPlanMenu recipeId={recipeId} anchorRef={ref} onClose={() => setOpen(false)} />}
    </>
  );
}

export default function LibraryCard({ recipe, detail, stats, reason }: LibraryCardProps) {
  const minutes = recipe.prep_time_minutes + recipe.cook_time_minutes;
  const eyebrow = [recipe.cuisine_type, minutes > 0 && `${minutes} min`].filter(Boolean).join(' · ');
  return (
    <article className="lib-card">
      <Link to={`/recipe/${recipe.id}`} className="lib-card-link">
        {recipe.image_url
          ? <img className="lib-card-img" src={recipe.image_url} alt="" loading="lazy" />
          : <div className="lib-card-img lib-card-img-empty" aria-hidden="true"><UtensilsCrossed size={32} /></div>}
        <div className="lib-card-body">
          {eyebrow && <span className="lib-card-eyebrow">{eyebrow}</span>}
          <span className="lib-card-title">{recipe.title}</span>
          {detail === 'description'
            ? recipe.description && <span className="lib-card-desc">{recipe.description}</span>
            : stats && (
              <span className="lib-card-rating">
                {stats.avg_rating != null && <><Star size={14} aria-hidden="true" />{stats.avg_rating.toFixed(1).replace(/\.0$/, '')}/10 · </>}
                cooked {stats.count === 1 ? 'once' : `${stats.count}×`}
              </span>
            )}
          {reason && <span className="lib-card-reason">{reason}</span>}
        </div>
      </Link>
      <PlanButton recipeId={recipe.id} title={recipe.title} />
    </article>
  );
}
