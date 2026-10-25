import { useEffect, useEffectEvent, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, Plus, Star, UtensilsCrossed } from 'lucide-react';
import { fetchRecipeImage, updateRecipeContent } from '../api/client';
import RecipeEditForm, { type RecipeContentEdit } from './RecipeEditForm';
import CookingChat from './CookingChat';
import RecipeHistory from './RecipeHistory';
import AddToPlanMenu from './AddToPlanMenu';
import Menu from './ui/Menu';
import Stepper from './ui/Stepper';
import { errorMessage } from '../hooks/useFetch';
import { useEatCounts } from '../hooks/useEatCounts';
import { useInventory } from '../hooks/useInventory';
import { matchIngredients, stockSummary } from '../utils/inventoryMatch';
import { scaleAmount, servingsLabel } from '../utils/format';
import type { Recipe } from '../types';

interface RecipeDetailProps {
  recipe: Recipe;
  onDelete: () => void;
}

export default function RecipeDetail({ recipe: initialRecipe, onDelete }: RecipeDetailProps) {
  // Local copy so edits and image refreshes show immediately.
  const [recipe, setRecipe] = useState(initialRecipe);
  const [servings, setServings] = useState(initialRecipe.servings || 1);
  const inventory = useInventory();
  const matched = matchIngredients(recipe.ingredients, inventory);
  const summary = stockSummary(matched);
  const stats = useEatCounts()[recipe.id];
  const [fetchingImage, setFetchingImage] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editSaving, setEditSaving] = useState(false);
  const [showPlanMenu, setShowPlanMenu] = useState(false);
  const addToPlanBtnRef = useRef<HTMLButtonElement>(null);

  const loadMissingImage = useEffectEvent((signal: AbortSignal) => {
    if (recipe.image_url) return;
    fetchRecipeImage(recipe.id)
      .then(result => { if (!signal.aborted) setRecipe(r => ({ ...r, image_url: result.image_url })); })
      .catch(() => {});
  });

  useEffect(() => {
    const controller = new AbortController();
    loadMissingImage(controller.signal);
    return () => controller.abort();
  }, [initialRecipe.id]);

  const refreshImage = async () => {
    setRecipe(r => ({ ...r, image_url: '' }));
    setFetchingImage(true);
    try {
      const result = await fetchRecipeImage(recipe.id);
      setRecipe(r => ({ ...r, image_url: result.image_url }));
    } catch (err) {
      alert('Could not fetch a new image: ' + errorMessage(err));
    } finally {
      setFetchingImage(false);
    }
  };

  const saveEdit = async ({ ingredients, instructions, cuisine_type }: RecipeContentEdit) => {
    setEditSaving(true);
    try {
      await updateRecipeContent(recipe.id, ingredients, instructions, cuisine_type);
      setRecipe(r => ({ ...r, ingredients, instructions, cuisine_type }));
      setEditing(false);
    } catch (err) {
      alert('Failed to save: ' + errorMessage(err));
    } finally {
      setEditSaving(false);
    }
  };

  const minutes = recipe.prep_time_minutes + recipe.cook_time_minutes;
  const factor = servings / (recipe.servings || 1);
  const eyebrow = [recipe.cuisine_type, minutes > 0 && `${minutes} min`, recipe.difficulty].filter(Boolean).join(' · ');

  return (
    <article className="rp">
      <Link to="/library" className="back-link"><ArrowLeft size={16} aria-hidden="true" />Library</Link>

      <header className="rp-hero">
        {recipe.image_url
          ? <img className="rp-hero-img" src={recipe.image_url} alt={recipe.title} />
          : (
            <div className="rp-hero-img rp-hero-empty" aria-hidden="true">
              <UtensilsCrossed size={40} />
              {fetchingImage && <span>Finding a photo…</span>}
            </div>
          )}
        <div className="rp-head">
          {eyebrow && <p className="rp-eyebrow">{eyebrow}</p>}
          <h1>{recipe.title}</h1>
          {recipe.description && <p className="rp-desc">{recipe.description}</p>}
          <dl className="rp-facts">
            {recipe.prep_time_minutes > 0 && <div><dt>Prep</dt><dd>{recipe.prep_time_minutes} min</dd></div>}
            {recipe.cook_time_minutes > 0 && <div><dt>Cook</dt><dd>{recipe.cook_time_minutes} min</dd></div>}
            <div>
              <dt>Your rating</dt>
              <dd>
                {stats?.avg_rating != null
                  ? <><Star size={14} aria-hidden="true" className="rp-star" />{stats.avg_rating.toFixed(1).replace(/\.0$/, '')}/10 · cooked {stats.count}×</>
                  : stats?.count ? `Cooked ${stats.count}×` : 'Not cooked yet'}
              </dd>
            </div>
          </dl>
          {recipe.dietary_restrictions?.some(d => d && d !== 'none') && (
            <div className="rp-chips">
              {recipe.dietary_restrictions.filter(d => d && d !== 'none').map(d => <span key={d} className="chip chip-success">{d}</span>)}
            </div>
          )}
          <div className="rp-actions">
            <button ref={addToPlanBtnRef} type="button" className="btn btn-primary" onClick={() => setShowPlanMenu(v => !v)}>
              <Plus size={16} aria-hidden="true" />Add to plan
            </button>
            <Menu label="More actions" items={[
              { label: 'Edit recipe', onSelect: () => setEditing(true) },
              { label: fetchingImage ? 'Finding a photo…' : 'Find a different photo', onSelect: refreshImage },
              { label: 'Delete recipe', danger: true, onSelect: onDelete },
            ]} />
            {showPlanMenu && (
              <AddToPlanMenu recipeId={recipe.id} anchorRef={addToPlanBtnRef} onClose={() => setShowPlanMenu(false)} />
            )}
          </div>
        </div>
      </header>

      {editing ? (
        <section className="rp-card">
          <RecipeEditForm recipe={recipe} onSave={saveEdit} onCancel={() => setEditing(false)} saving={editSaving} />
        </section>
      ) : (
        <div className="rp-body">
          <section className="rp-card rp-ingredients" aria-labelledby="rp-ingredients">
            <div className="rp-card-head">
              <h2 id="rp-ingredients">Ingredients</h2>
              <Stepper label="Servings" value={servings} onChange={setServings} max={24} format={servingsLabel} />
            </div>
            {summary && (
              <p className="rp-stock">
                {summary.missing === 0 ? 'Everything is in your pantry' : `${summary.inStock} of ${summary.total} in your pantry`}
              </p>
            )}
            <ul className="rp-ingredient-list">
              {recipe.ingredients?.map((ing, i) => (
                <li key={i}>
                  <span className="rp-amount">{[scaleAmount(ing.amount, factor, ing.unit), ing.unit].filter(Boolean).join(' ')}</span>
                  <span className="rp-ingredient">
                    {ing.name}
                    {ing.notes && <span className="ingredient-notes"> ({ing.notes})</span>}
                  </span>
                  {matched && (
                    <span className={`stock-dot ${matched[i].inStock ? 'stock-dot-in' : 'stock-dot-out'}`}
                      title={matched[i].inStock ? 'In your pantry' : 'Not in your pantry'} />
                  )}
                </li>
              ))}
            </ul>
          </section>

          <div className="rp-main">
            <section className="rp-card" aria-labelledby="rp-method">
              <h2 id="rp-method">Method</h2>
              <ol className="rp-steps">
                {recipe.instructions?.map((step, i) => <li key={i}>{step}</li>)}
              </ol>
              {(recipe.tags?.length > 0 || recipe.generated_by_model) && (
                <div className="rp-meta">
                  {recipe.tags?.map(tag => <span key={tag} className="tag">{tag}</span>)}
                  {recipe.generated_by_model && <span className="rp-model">Generated by {recipe.generated_by_model}</span>}
                </div>
              )}
            </section>

            <section className="rp-card" aria-labelledby="rp-history">
              <h2 id="rp-history">Cook history</h2>
              <RecipeHistory recipeId={recipe.id} />
            </section>

            <details className="rp-card rp-chat">
              <summary>Ask about this recipe</summary>
              <CookingChat recipeId={recipe.id} />
            </details>
          </div>
        </div>
      )}
    </article>
  );
}
