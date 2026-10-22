import { useState } from 'react';
import { matchIngredients, stockSummary } from '../../utils/inventoryMatch';
import type { InventoryItem, Recipe } from '../../types';

interface BrowseRecipeCardProps {
  recipe: Recipe;
  servings: number;
  onServingsChange: (servings: number) => void;
  onAdd: () => void;
  adding: boolean;
  inventory: InventoryItem[];
}

export default function BrowseRecipeCard({ recipe, servings, onServingsChange, onAdd, adding, inventory }: BrowseRecipeCardProps) {
  const [expanded, setExpanded] = useState(false);
  const matched = matchIngredients(recipe.ingredients, inventory);
  const summary = stockSummary(matched);

  return (
    <div className="plan-browse-item plan-browse-card">
      <div className="plan-browse-card-main">
        {recipe.image_url && (
          <img className="plan-browse-thumb" src={recipe.image_url} alt={recipe.title} loading="lazy" />
        )}
        <div className="plan-browse-info">
          <div className="plan-browse-title-row">
            <strong>{recipe.title}</strong>
            {recipe.cuisine_type && <span className="cuisine-badge">{recipe.cuisine_type}</span>}
          </div>
          {recipe.description && (
            <p className="plan-browse-description">
              {recipe.description.length > 120 ? recipe.description.slice(0, 120) + '…' : recipe.description}
            </p>
          )}
          <div className="plan-browse-meta-row">
            {recipe.prep_time_minutes > 0 && <span>Prep {recipe.prep_time_minutes}m</span>}
            {recipe.cook_time_minutes > 0 && <span>Cook {recipe.cook_time_minutes}m</span>}
            {recipe.difficulty && <span className={`difficulty difficulty-${recipe.difficulty}`}>{recipe.difficulty}</span>}
            {summary && (
              <span className={`stock-badge ${summary.missing === 0 ? 'stock-badge-all' : summary.inStock === 0 ? 'stock-badge-none' : 'stock-badge-partial'}`}>
                {summary.missing === 0 ? 'all in stock' : `${summary.missing} missing`}
              </span>
            )}
            <button
              className="btn-link"
              onClick={() => setExpanded(e => !e)}
              aria-expanded={expanded}
            >
              {expanded ? 'Hide details' : 'Show details'}
            </button>
          </div>
        </div>
        <div className="plan-browse-actions">
          <input
            type="number"
            min="1"
            max="20"
            value={servings}
            onChange={e => onServingsChange(parseInt(e.target.value) || 1)}
            className="servings-input"
            aria-label="Servings"
          />
          <button className="btn btn-primary" onClick={onAdd} disabled={adding}>
            {adding ? 'Adding…' : 'Add'}
          </button>
        </div>
      </div>
      {expanded && (
        <div className="plan-browse-expanded">
          {recipe.ingredients && recipe.ingredients.length > 0 && (
            <div className="plan-browse-ingredients">
              <h5>Ingredients</h5>
              <ul>
                {recipe.ingredients.map((ing, i) => (
                  <li key={i} className="ingredient-row">
                    <span><strong>{ing.amount} {ing.unit}</strong> {ing.name}</span>
                    {matched && <span className={`stock-dot ${matched[i].inStock ? 'stock-dot-in' : 'stock-dot-out'}`} title={matched[i].inStock ? 'In stock' : 'Not in stock'} />}
                  </li>
                ))}
              </ul>
            </div>
          )}
          {recipe.instructions && recipe.instructions.length > 0 && (
            <div className="plan-browse-instructions">
              <h5>Instructions</h5>
              <ol>
                {recipe.instructions.map((step, i) => <li key={i}>{step}</li>)}
              </ol>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
