import { useState } from 'react';
import { deleteRecipe } from '../api/client';

function RecipeCompareCard({ recipe, selected, onToggle }) {
  const ingredientNames = (recipe.ingredients || []).slice(0, 5).map(i => i.name);
  const created = new Date(recipe.created_at).toLocaleDateString(undefined, {
    year: 'numeric', month: 'short', day: 'numeric',
  });

  return (
    <div
      className={`dedup-card ${selected ? 'dedup-card--selected' : ''}`}
      onClick={onToggle}
      role="checkbox"
      aria-checked={selected}
      tabIndex={0}
      onKeyDown={e => { if (e.key === ' ' || e.key === 'Enter') onToggle(); }}
    >
      {recipe.image_url && (
        <img className="dedup-card-img" src={recipe.image_url} alt="" />
      )}
      <div className="dedup-card-body">
        <div className="dedup-card-title">{recipe.title}</div>
        {recipe.description && (
          <p className="dedup-card-desc">
            {recipe.description.length > 120
              ? recipe.description.slice(0, 120) + '…'
              : recipe.description}
          </p>
        )}
        <div className="dedup-card-meta">
          {recipe.cuisine_type && <span className="tag">{recipe.cuisine_type}</span>}
          {recipe.difficulty && <span className="tag">{recipe.difficulty}</span>}
        </div>
        {ingredientNames.length > 0 && (
          <p className="dedup-card-ingredients">
            {ingredientNames.join(', ')}{recipe.ingredients.length > 5 ? ', …' : ''}
          </p>
        )}
        <p className="dedup-card-date">Added {created}</p>
      </div>
      <div className={`dedup-card-checkbox ${selected ? 'dedup-card-checkbox--checked' : ''}`}>
        {selected ? '✓ Delete' : 'Keep'}
      </div>
    </div>
  );
}

export default function DuplicatesModal({ groups, onClose, onDeleted }) {
  const [selected, setSelected] = useState(new Set());
  const [deleting, setDeleting] = useState(false);

  const toggle = (id) => {
    setSelected(prev => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  };

  const handleDelete = async () => {
    if (selected.size === 0) return;
    setDeleting(true);
    try {
      await Promise.all([...selected].map(id => deleteRecipe(id)));
      onDeleted([...selected]);
    } catch (err) {
      alert('Failed to delete some recipes: ' + err.message);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={e => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="modal dedup-modal">
        <div className="modal-header">
          <h3>Duplicate Recipes</h3>
          <button className="modal-close" onClick={onClose} aria-label="Close">×</button>
        </div>

        <div className="dedup-modal-body">
          {groups.length === 0 ? (
            <p className="empty-state">No duplicates found.</p>
          ) : (
            <>
              <p className="dedup-modal-hint">
                Click a recipe card to mark it for deletion. You can mark multiple recipes across groups.
              </p>
              {groups.map((group, gi) => (
                <div key={gi} className="dedup-group">
                  <div className="dedup-group-label">Group {gi + 1} of {groups.length}</div>
                  <div className="dedup-group-cards">
                    {group.map(recipe => (
                      <RecipeCompareCard
                        key={recipe.id}
                        recipe={recipe}
                        selected={selected.has(recipe.id)}
                        onToggle={() => toggle(recipe.id)}
                      />
                    ))}
                  </div>
                </div>
              ))}
            </>
          )}
        </div>

        <div className="modal-footer">
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
          {groups.length > 0 && (
            <button
              className="btn btn-danger"
              onClick={handleDelete}
              disabled={selected.size === 0 || deleting}
            >
              {deleting ? 'Deleting…' : `Delete ${selected.size} selected`}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
