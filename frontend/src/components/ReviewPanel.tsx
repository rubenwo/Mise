import { useState } from 'react';
import { saveRecipe } from '../api/client';
import { errorMessage } from '../hooks/useFetch';
import type { GeneratedRecipe, Ingredient, Recipe } from '../types';
import RecipeCard from './RecipeCard';
import RecipeEditForm from './RecipeEditForm';

type ContentEdit = { ingredients: Ingredient[]; instructions: string[] };

interface ReviewPanelProps {
  recipes: GeneratedRecipe[];
  onRefine: (recipe: Recipe, feedback: string) => void;
  onRemove: (clientId: number) => void;
  loading: boolean;
}

export default function ReviewPanel({ recipes, onRefine, onRemove, loading }: ReviewPanelProps) {
  const [saving, setSaving] = useState<Record<number, boolean>>({});
  const [feedback, setFeedback] = useState<Record<number, string>>({});
  const [editingId, setEditingId] = useState<number | null>(null);
  // Local ingredient/instruction edits keyed by client id, layered over the
  // generated recipes so new generations still flow through.
  const [edits, setEdits] = useState<Record<number, ContentEdit>>({});
  const [revisions, setRevisions] = useState<Record<number, number>>({});

  const localRecipes = recipes.map(r => (edits[r._clientId] ? { ...r, ...edits[r._clientId] } : r));

  const handleSave = async (recipe: GeneratedRecipe) => {
    const clientId = recipe._clientId;
    setSaving(prev => ({ ...prev, [clientId]: true }));
    try {
      // Strip internal client-side fields before persisting.
      const { _warnings, _clientId, ...recipeData } = recipe;
      await saveRecipe(recipeData);
      onRemove(clientId);
    } catch (err) {
      alert('Failed to save: ' + errorMessage(err));
      setSaving(prev => ({ ...prev, [clientId]: false }));
    }
  };

  const handleRefine = (recipe: Recipe, clientId: number) => {
    const fb = feedback[clientId];
    if (!fb) return;
    onRefine(recipe, fb);
    setFeedback(prev => ({ ...prev, [clientId]: '' }));
  };

  const handleEditSave = (clientId: number, edit: ContentEdit) => {
    setEdits(prev => ({ ...prev, [clientId]: edit }));
    setRevisions(prev => ({ ...prev, [clientId]: (prev[clientId] || 0) + 1 }));
    setEditingId(null);
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

            {editingId === clientId ? (
              <RecipeEditForm
                recipe={recipe}
                onSave={({ ingredients, instructions }) => handleEditSave(clientId, { ingredients, instructions })}
                onCancel={() => setEditingId(null)}
                saving={false}
              />
            ) : (
              <>
                {recipe._warnings.length > 0 && (
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
                  <button className="btn btn-secondary" onClick={() => setEditingId(clientId)}>
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
