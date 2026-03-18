import { useState, useEffect } from 'react';
import { saveRecipe } from '../api/client';
import RecipeCard from './RecipeCard';

export default function ReviewPanel({ recipes, onRefine, onRemove, loading }) {
  const [saving, setSaving] = useState({});
  const [feedback, setFeedback] = useState({});

  useEffect(() => {
    setSaving({});
    setFeedback({});
  }, [recipes]);

  const handleSave = async (recipe, index) => {
    setSaving(prev => ({ ...prev, [index]: true }));
    try {
      await saveRecipe(recipe);
      onRemove(index);
    } catch (err) {
      alert('Failed to save: ' + err.message);
      setSaving(prev => ({ ...prev, [index]: false }));
    }
  };

  const handleRefine = (recipe, index) => {
    const fb = feedback[index];
    if (!fb) return;
    onRefine(recipe, fb);
    setFeedback(prev => ({ ...prev, [index]: '' }));
  };

  if (recipes.length === 0) return null;

  return (
    <div className="review-panel">
      <h3>Generated Recipes</h3>
      {recipes.map((recipe, i) => (
        <div key={i} className="review-item">
          <button type="button" className="review-dismiss" onClick={() => onRemove(i)} title="Dismiss">&times;</button>
          <RecipeCard recipe={recipe} showIngredients />
          <div className="review-actions">
            <button className="btn btn-primary" onClick={() => handleSave(recipe, i)} disabled={saving[i]}>
              {saving[i] ? 'Saving...' : 'Save Recipe'}
            </button>
            <div className="refine-section">
              <input
                type="text"
                placeholder="What would you like to change?"
                value={feedback[i] || ''}
                onChange={e => setFeedback(prev => ({ ...prev, [i]: e.target.value }))}
              />
              <button className="btn btn-secondary" onClick={() => handleRefine(recipe, i)} disabled={loading || !feedback[i]}>
                Refine
              </button>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
