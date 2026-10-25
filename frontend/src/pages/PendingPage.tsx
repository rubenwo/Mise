import { useState, useEffect, useRef } from 'react';
import {
  listPendingRecipes, approvePendingRecipe, rejectPendingRecipe,
  getSettings, updatePendingRecipeContent, getFeatureStatus,
  previewImageByTitle,
} from '../api/client';
import RecipeEditForm, { type RecipeContentEdit } from '../components/RecipeEditForm';
import { errorMessage, useFetch } from '../hooks/useFetch';
import type { Recipe, SSEEvent } from '../types';

function PendingThumb({ recipe }: { recipe: Recipe }) {
  const preview = useFetch(
    !recipe.image_url && recipe.title ? `preview-image:${recipe.title}` : null,
    signal => previewImageByTitle(recipe.title, signal),
  );
  const src = recipe.image_url || preview.data?.image_url;

  if (src) {
    return <img className="pending-item-thumb" src={src} alt={recipe.title} loading="lazy" />;
  }
  const letter = (recipe.title || '?').trim().charAt(0).toUpperCase();
  return <div className="pending-item-thumb-placeholder" aria-hidden="true">{letter}</div>;
}

type Acting = 'approving' | 'rejecting' | null;

interface PendingItemProps {
  recipe: Recipe;
  onApprove: (id: number) => void;
  onReject: (id: number) => void;
  onEdit: (id: number) => void;
  editing: boolean;
  onEditSave: (edit: RecipeContentEdit) => void;
  onEditCancel: () => void;
  editSaving: boolean;
  acting: Acting | undefined;
}

function PendingItem({ recipe, onApprove, onReject, onEdit, editing, onEditSave, onEditCancel, editSaving, acting }: PendingItemProps) {
  const [expanded, setExpanded] = useState(false);

  if (editing) {
    return (
      <div className="pending-item">
        <div className="pending-item-expanded" style={{ borderTop: 'none' }}>
          <RecipeEditForm
            recipe={recipe}
            onSave={onEditSave}
            onCancel={onEditCancel}
            saving={editSaving}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="pending-item">
      <div className="pending-item-row" onClick={() => setExpanded(e => !e)}>
        <PendingThumb recipe={recipe} />
        <div className="pending-item-body">
          <div className="pending-item-title-row">
            <span className="pending-item-title">{recipe.title}</span>
            {recipe.cuisine_type && <span className="cuisine-badge">{recipe.cuisine_type}</span>}
          </div>
          {recipe.description && (
            <p className="pending-item-desc">{recipe.description}</p>
          )}
          <div className="pending-item-meta">
            {recipe.prep_time_minutes > 0 && <span>Prep {recipe.prep_time_minutes}m</span>}
            {recipe.cook_time_minutes > 0 && <span>Cook {recipe.cook_time_minutes}m</span>}
            {recipe.servings > 0 && <span>{recipe.servings} servings</span>}
            {recipe.difficulty && <span className={`difficulty difficulty-${recipe.difficulty}`}>{recipe.difficulty}</span>}
            <span style={{ marginLeft: 'auto' }}>
              {expanded ? '▲ collapse' : '▼ expand'}
            </span>
          </div>
        </div>
        <div className="pending-item-quick" onClick={e => e.stopPropagation()}>
          <button
            className="btn btn-primary btn-sm"
            onClick={() => onApprove(recipe.id)}
            disabled={!!acting}
          >
            {acting === 'approving' ? 'Saving…' : 'Save'}
          </button>
          <button
            className="btn btn-danger btn-sm"
            onClick={() => onReject(recipe.id)}
            disabled={!!acting}
          >
            {acting === 'rejecting' ? '…' : 'Reject'}
          </button>
        </div>
      </div>

      {expanded && (
        <div className="pending-item-expanded">
          {recipe.ingredients && recipe.ingredients.length > 0 && (
            <div className="recipe-card-ingredients" style={{ marginTop: 0 }}>
              <h4>Ingredients</h4>
              <ul className="ingredients-list">
                {recipe.ingredients.map((ing, i) => (
                  <li key={i}>
                    <strong>{ing.amount} {ing.unit}</strong> {ing.name}
                    {ing.notes && <span className="ingredient-notes"> ({ing.notes})</span>}
                  </li>
                ))}
              </ul>
            </div>
          )}
          {recipe.instructions && recipe.instructions.length > 0 && (
            <div className="recipe-card-instructions">
              <h4>Instructions</h4>
              <ol className="instructions-list">
                {recipe.instructions.map((step, i) => <li key={i}>{step}</li>)}
              </ol>
            </div>
          )}
          {recipe.tags && recipe.tags.length > 0 && (
            <div className="recipe-tags" style={{ marginTop: 12 }}>
              {recipe.tags.map(tag => <span key={tag} className="tag">{tag}</span>)}
            </div>
          )}
          <p className="pending-item-date">
            Generated {new Date(recipe.created_at).toLocaleDateString(undefined, { dateStyle: 'medium' })}
          </p>
          <div className="pending-item-actions">
            <button className="btn btn-secondary" onClick={() => onEdit(recipe.id)}>
              Edit ingredients & instructions
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export default function PendingPage({ onCountChange }: { onCountChange?: (count: number) => void }) {
  const pending = useFetch('pending', listPendingRecipes);
  const setRecipes = pending.setData;
  const recipes = pending.data ?? [];
  const settings = useFetch('settings', getSettings);
  const features = useFetch('features', getFeatureStatus);
  const bgEnabled = settings.data?.find(s => s.key === 'background_generation_enabled')?.value === 'true';
  const bgGenStatus = features.data ? (features.data['background-generation'] ?? 'unconfigured') : null;

  const [acting, setActing] = useState<Record<number, Acting>>({});
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editSaving, setEditSaving] = useState(false);
  const [generatingMsg, setGeneratingMsg] = useState<string | null>(null);
  const generatingTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  // Keep the nav badge in sync with the list once it has loaded.
  const count = pending.data?.length;
  useEffect(() => {
    if (count !== undefined) onCountChange?.(count);
  }, [count, onCountChange]);

  useEffect(() => {
    const es = new EventSource('/api/pending/events');

    es.onmessage = (e) => {
      let ev: SSEEvent;
      try { ev = JSON.parse(e.data); } catch { return; }

      if (ev.type === 'pending_added') {
        const recipe = ev.data as Recipe;
        setRecipes(rs => (rs?.some(r => r.id === recipe.id) ? rs : [recipe, ...(rs ?? [])]));
        setGeneratingMsg(null);
        clearTimeout(generatingTimerRef.current);
      } else if (ev.type === 'status' || ev.type === 'tool') {
        setGeneratingMsg(ev.message || 'Generating recipe…');
        clearTimeout(generatingTimerRef.current);
        generatingTimerRef.current = setTimeout(() => setGeneratingMsg(null), 60_000);
      }
    };

    return () => {
      es.close();
      clearTimeout(generatingTimerRef.current);
    };
  }, [setRecipes]);

  const remove = (id: number) => setRecipes(rs => rs?.filter(r => r.id !== id));

  const act = async (id: number, kind: Exclude<Acting, null>, action: () => Promise<unknown>, failure: string) => {
    setActing(a => ({ ...a, [id]: kind }));
    try {
      await action();
      remove(id);
    } catch (err) {
      alert(`${failure}: ${errorMessage(err)}`);
    } finally {
      setActing(a => ({ ...a, [id]: null }));
    }
  };

  const handleApprove = (id: number) => act(id, 'approving', () => approvePendingRecipe(id), 'Failed to save');
  const handleReject = (id: number) => act(id, 'rejecting', () => rejectPendingRecipe(id), 'Failed to reject');

  const handleEditSave = async (id: number, { ingredients, instructions }: Pick<RecipeContentEdit, 'ingredients' | 'instructions'>) => {
    setEditSaving(true);
    try {
      await updatePendingRecipeContent(id, ingredients, instructions);
      setRecipes(rs => rs?.map(r => r.id === id ? { ...r, ingredients, instructions } : r));
      setEditingId(null);
    } catch (err) {
      alert('Failed to save: ' + errorMessage(err));
    } finally {
      setEditSaving(false);
    }
  };

  if (pending.loading && !pending.data) return <p className="empty-state">Loading...</p>;

  return (
    <div className="pending-page">
      <h1>Recipes to review</h1>
      <p className="pending-page-hint">
        Recipes generated in the background, waiting for you. Save the ones you like or reject the rest.
        Discarded automatically after 7 days.
      </p>
      {generatingMsg && (
        <div className="pending-generating-banner">
          <span className="pending-generating-spinner" />
          {generatingMsg}
        </div>
      )}
      {bgEnabled && bgGenStatus && bgGenStatus !== 'available' && (
        <div className="settings-warning">
          {bgGenStatus === 'offline'
            ? 'Background generation is enabled but the provider tagged background-generation is currently offline. No new recipes will be generated until it comes back.'
            : 'Background generation is enabled but no provider is tagged background-generation. Assign the tag in Settings → LLM Providers.'}
        </div>
      )}

      {recipes.length === 0 ? (
        <p className="empty-state">
          {bgEnabled
            ? 'No pending recipes yet. More will appear here automatically.'
            : 'No pending recipes. Enable background generation in Settings to get some.'}
        </p>
      ) : (
        <div className="pending-list">
          {recipes.map(recipe => (
            <PendingItem
              key={recipe.id}
              recipe={recipe}
              acting={acting[recipe.id]}
              editing={editingId === recipe.id}
              editSaving={editSaving}
              onApprove={handleApprove}
              onReject={handleReject}
              onEdit={(id) => setEditingId(id)}
              onEditSave={({ ingredients, instructions }) => handleEditSave(recipe.id, { ingredients, instructions })}
              onEditCancel={() => setEditingId(null)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
