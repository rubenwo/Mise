import { useState, useEffect } from 'react';
import { saveRecipe } from '../api/client';
import RecipeCard from './RecipeCard';
import RecipeEditForm from './RecipeEditForm';

export default function ReviewPanel({ recipes, onRefine, onRemove, loading }) {
  const [saving, setSaving] = useState({});
  const [feedback, setFeedback] = useState({});
  const [editingIndex, setEditingIndex] = useState(null);
  const [localRecipes, setLocalRecipes] = useState([]);
  const [revisions, setRevisions] = useState({});

  useEffect(() => {
    setLocalRecipes(recipes);
  }, [recipes]);

  const handleSave = async (recipe) => {
    const clientId = recipe._clientId;
    setSaving(prev => ({ ...prev, [clientId]: true }));
    try {
      // Strip internal client-side fields before persisting.
      const { _warnings, _clientId, ...recipeData } = recipe;
      await saveRecipe(recipeData);
      onRemove(clientId);
    } catch (err) {
      alert('Failed to save: ' + err.message);
      setSaving(prev => ({ ...prev, [clientId]: false }));
    }
  };

  const handleRefine = (recipe, clientId) => {
    const fb = feedback[clientId];
    if (!fb) return;
    onRefine(recipe, fb);
    setFeedback(prev => ({ ...prev, [clientId]: '' }));
  };

  const handleEditSave = (clientId, { ingredients, instructions }) => {
    setLocalRecipes(prev => prev.map(r =>
      r._clientId === clientId ? { ...r, ingredients, instructions } : r
    ));
    setRevisions(prev => ({ ...prev, [clientId]: (prev[clientId] || 0) + 1 }));
    setEditingIndex(null);
  };

  if (localRecipes.length === 0) return null;

  return (
    <div className="review-panel">
      <h3>Generated Recipes</h3>
      {localRecipes.map((recipe) => {
        const clientId = recipe._clientId;
        return (
          <div key={clientId} className="review-item">
            <button type="button" className="review-dismiss" onClick={() => onRemove(clientId)} title="Dismiss">&times;</button>

            {editingIndex === clientId ? (
              <RecipeEditForm
                recipe={recipe}
                onSave={({ ingredients, instructions }) => handleEditSave(clientId, { ingredients, instructions })}
                onCancel={() => setEditingIndex(null)}
                saving={false}
              />
            ) : (
              <>
                {recipe._warnings && recipe._warnings.length > 0 && (
                  <div className="near-duplicate-warning">
                    <span className="near-duplicate-icon">⚠</span>
                    <span>Similar to existing recipe{recipe._warnings.length > 1 ? 's' : ''}: </span>
                    {recipe._warnings.map((w, wi) => (
                      <span key={wi}>
                        {wi > 0 && ', '}
                        <a href={`/recipe/${w.id}`} target="_blank" rel="noopener noreferrer">{w.title}</a>
                      </span>
                    ))}
                  </div>
                )}
                <RecipeCard key={revisions[clientId] || 0} recipe={recipe} showIngredients showInstructions />
                <div className="review-actions">
                  <button className="btn btn-primary" onClick={() => handleSave(recipe)} disabled={saving[clientId]}>
                    {saving[clientId] ? 'Saving...' : 'Save Recipe'}
                  </button>
                  <button className="btn btn-secondary" onClick={() => setEditingIndex(clientId)}>
                    Edit
                  </button>
                  <div className="refine-section">
                    <input
                      type="text"
                      placeholder="What would you like to change?"
                      value={feedback[clientId] || ''}
                      onChange={e => setFeedback(prev => ({ ...prev, [clientId]: e.target.value }))}
                    />
                    <button className="btn btn-secondary" onClick={() => handleRefine(recipe, clientId)} disabled={loading || !feedback[clientId]}>
                      Refine
                    </button>
                  </div>
                </div>
              </>
            )}
          </div>
        );
      })}
    </div>
  );
}
