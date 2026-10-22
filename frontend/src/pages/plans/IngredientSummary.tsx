import { useState } from 'react';
import { getPlanSuggestions, saveRecipe } from '../../api/client';
import AHOrderModal from '../../components/AHOrderModal';
import RecipeCard from '../../components/RecipeCard';
import { errorMessage } from '../../hooks/useFetch';
import type { AggregatedIngredient, PlanSuggestions } from '../../types';

interface IngredientSummaryProps {
  ingredients: AggregatedIngredient[];
  planId: number;
}

export default function IngredientSummary({ ingredients, planId }: IngredientSummaryProps) {
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});
  const [selected, setSelected] = useState<Record<number, boolean>>({});
  const [suggestions, setSuggestions] = useState<PlanSuggestions | null>(null);
  const [loadingSuggestions, setLoadingSuggestions] = useState(false);
  const [savedGenerated, setSavedGenerated] = useState(false);
  const [showAHModal, setShowAHModal] = useState(false);

  const selectedNames = ingredients.filter((_, i) => selected[i]).map(ing => ing.name);

  const handleFindRecipes = async () => {
    if (selectedNames.length === 0) return;
    setLoadingSuggestions(true);
    setSuggestions(null);
    setSavedGenerated(false);
    try {
      const data = await getPlanSuggestions(planId, selectedNames);
      setSuggestions(data);
    } catch (err) {
      alert('Failed to get suggestions: ' + errorMessage(err));
    } finally {
      setLoadingSuggestions(false);
    }
  };

  const handleSaveGenerated = async () => {
    if (!suggestions?.generated_recipe) return;
    try {
      await saveRecipe(suggestions.generated_recipe);
      setSavedGenerated(true);
    } catch (err) {
      alert('Failed to save: ' + errorMessage(err));
    }
  };

  return (
    <div className="ingredient-summary">
      <div className="ingredient-summary-header">
        <h4>Shopping List ({ingredients.length} items)</h4>
        <button className="btn btn-secondary btn-sm ah-order-btn" onClick={() => setShowAHModal(true)}>
          Order at Albert Heijn
        </button>
      </div>
      <p className="ingredient-summary-hint">Select leftover ingredients to find recipes that use them</p>
      {showAHModal && <AHOrderModal planId={planId} onClose={() => setShowAHModal(false)} />}
      <ul className="ingredient-summary-list">
        {ingredients.map((ing, i) => (
          <li key={i} className="ingredient-summary-item">
            <div className="ingredient-summary-main">
              <input
                type="checkbox"
                checked={!!selected[i]}
                onChange={() => setSelected(prev => ({ ...prev, [i]: !prev[i] }))}
              />
              <span className="ingredient-summary-amount">{ing.amount} {ing.unit}</span>
              <span className="ingredient-summary-name" onClick={() => setExpanded(prev => ({ ...prev, [i]: !prev[i] }))}>{ing.name}</span>
              <span className="ingredient-summary-count">{ing.recipes.length} recipe{ing.recipes.length > 1 ? 's' : ''}</span>
            </div>
            {expanded[i] && (
              <div className="ingredient-summary-recipes">
                {ing.recipes.map((r, j) => <span key={j} className="tag">{r}</span>)}
              </div>
            )}
          </li>
        ))}
      </ul>

      {selectedNames.length > 0 && (
        <button
          className="btn btn-primary ingredient-suggest-btn"
          onClick={handleFindRecipes}
          disabled={loadingSuggestions}
        >
          {loadingSuggestions ? 'Searching...' : `Find recipes for ${selectedNames.length} ingredient${selectedNames.length > 1 ? 's' : ''}`}
        </button>
      )}

      {suggestions && (
        <div className="suggestions-panel">
          <h4>Suggestions</h4>
          {(suggestions.db_recipes || []).length > 0 && (
            <div className="suggestions-section">
              <h5>From your library</h5>
              {suggestions.db_recipes.map(r => (
                <div key={r.id} className="suggestion-item">
                  <RecipeCard recipe={r} showLink />
                </div>
              ))}
            </div>
          )}
          {suggestions.generated_recipe && (
            <div className="suggestions-section">
              <h5>Generated recipe</h5>
              <div className="suggestion-item">
                <RecipeCard recipe={suggestions.generated_recipe} showIngredients />
                {savedGenerated ? (
                  <span className="saved-badge">Saved!</span>
                ) : (
                  <button className="btn btn-primary" onClick={handleSaveGenerated}>Save Recipe</button>
                )}
              </div>
            </div>
          )}
          {(suggestions.db_recipes || []).length === 0 && !suggestions.generated_recipe && (
            <p className="empty-state">No suggestions found for these ingredients.</p>
          )}
        </div>
      )}
    </div>
  );
}
