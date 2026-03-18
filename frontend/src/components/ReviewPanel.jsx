import { useState } from 'react';
import { saveRecipe } from '../api/client';
import RecipeCard from './RecipeCard';

export default function ReviewPanel({ recipes, onRefine, loading }) {
  const [saving, setSaving] = useState({});
  const [saved, setSaved] = useState({});
  const [feedback, setFeedback] = useState({});

  const handleSave = async (recipe, index) => {
    setSaving(prev => ({ ...prev, [index]: true }));
    try {
      await saveRecipe(recipe);
      setSaved(prev => ({ ...prev, [index]: true }));
    } catch (err) {
      alert('Failed to save: ' + err.message);
    } finally {
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
          <RecipeCard recipe={recipe} />
          <div className="review-actions">
            {saved[i] ? (
              <span className="saved-badge">Saved!</span>
            ) : (
              <button className="btn btn-primary" onClick={() => handleSave(recipe, i)} disabled={saving[i]}>
                {saving[i] ? 'Saving...' : 'Save Recipe'}
              </button>
            )}
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
