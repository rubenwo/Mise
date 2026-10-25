import { useState } from 'react';
import { getPlanSuggestions, saveRecipe } from '../../api/client';
import RecipeCard from '../../components/RecipeCard';
import { errorMessage } from '../../hooks/useFetch';
import type { AggregatedIngredient, PlanSuggestions } from '../../types';

interface LeftoversModalProps {
  planId: number;
  ingredients: AggregatedIngredient[];
  onClose: () => void;
}

/** Pick ingredients you'll have left over and find (or generate) recipes that use them. */
export default function LeftoversModal({ planId, ingredients, onClose }: LeftoversModalProps) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [suggestions, setSuggestions] = useState<PlanSuggestions | null>(null);
  const [loading, setLoading] = useState(false);
  const [saved, setSaved] = useState(false);

  const toggle = (name: string) => setSelected(prev => {
    const s = new Set(prev);
    if (s.has(name)) s.delete(name); else s.add(name);
    return s;
  });

  const find = async () => {
    setLoading(true);
    setSuggestions(null);
    setSaved(false);
    try {
      setSuggestions(await getPlanSuggestions(planId, [...selected]));
    } catch (err) {
      alert('Failed to get suggestions: ' + errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const saveGenerated = async () => {
    if (!suggestions?.generated_recipe) return;
    try {
      await saveRecipe(suggestions.generated_recipe);
      setSaved(true);
    } catch (err) {
      alert('Failed to save: ' + errorMessage(err));
    }
  };

  const dbRecipes = suggestions?.db_recipes ?? [];

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" role="dialog" aria-labelledby="leftovers-title" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <h3 id="leftovers-title">Use up leftovers</h3>
          <button className="modal-close" onClick={onClose} aria-label="Close">&times;</button>
        </div>
        <div className="modal-body">
          {!suggestions && (
            <>
              <p className="modal-intro">Pick what you expect to have left over, and Mise finds recipes that use it.</p>
              <div className="leftover-options">
                {ingredients.map(i => (
                  <label key={i.name} className={`chip chip-toggle${selected.has(i.name) ? ' chip-on' : ''}`}>
                    <input type="checkbox" checked={selected.has(i.name)} onChange={() => toggle(i.name)} />
                    {i.name}
                  </label>
                ))}
              </div>
              <div className="modal-footer">
                <button className="btn btn-primary" onClick={find} disabled={selected.size === 0 || loading}>
                  {loading ? 'Searching…' : selected.size === 0 ? 'Find recipes' : `Find recipes using ${selected.size} ingredient${selected.size === 1 ? '' : 's'}`}
                </button>
              </div>
            </>
          )}
          {suggestions && (
            <>
              {dbRecipes.length > 0 && (
                <section className="suggestions-section">
                  <h4>From your library</h4>
                  {dbRecipes.map(r => <RecipeCard key={r.id} recipe={r} showLink />)}
                </section>
              )}
              {suggestions.generated_recipe && (
                <section className="suggestions-section">
                  <h4>New idea</h4>
                  <RecipeCard recipe={suggestions.generated_recipe} showIngredients />
                  {saved
                    ? <span className="saved-badge">Saved to library</span>
                    : <button className="btn btn-primary" onClick={saveGenerated}>Save recipe</button>}
                </section>
              )}
              {dbRecipes.length === 0 && !suggestions.generated_recipe && (
                <p className="empty-state">No recipes found for these ingredients.</p>
              )}
              <div className="modal-footer">
                <button className="btn btn-secondary" onClick={() => setSuggestions(null)}>Pick again</button>
                <button className="btn btn-primary" onClick={onClose}>Done</button>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
