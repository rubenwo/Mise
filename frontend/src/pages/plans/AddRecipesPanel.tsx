import { useEffect, useState } from 'react';
import { librarySearch } from '../../api/client';
import { useFetch } from '../../hooks/useFetch';
import type { InventoryItem, Recipe } from '../../types';
import BrowseRecipeCard from './BrowseRecipeCard';

interface AddRecipesPanelProps {
  planRecipeIds: Set<number>;
  inventory: InventoryItem[];
  onAdd: (recipe: Recipe, servings: number) => Promise<void>;
}

/** Searches the library and adds recipes to the plan. */
export default function AddRecipesPanel({ planRecipeIds, inventory, onAdd }: AddRecipesPanelProps) {
  const [filter, setFilter] = useState('');
  const [debounced, setDebounced] = useState('');
  const [servings, setServings] = useState<Record<number, number>>({});
  const [adding, setAdding] = useState<Record<number, boolean>>({});

  // Debounced backend search (immediate when cleared).
  useEffect(() => {
    const trimmed = filter.trim();
    const timer = setTimeout(() => setDebounced(trimmed), trimmed ? 300 : 0);
    return () => clearTimeout(timer);
  }, [filter]);

  const browse = useFetch(`plan-browse:${debounced}`, signal => librarySearch({ keywords: debounced, limit: 100 }, signal));
  const available = (browse.data?.recipes ?? []).filter(r => !planRecipeIds.has(r.id));
  const servingsFor = (r: Recipe) => servings[r.id] || r.servings || 4;

  const add = async (recipe: Recipe) => {
    setAdding(prev => ({ ...prev, [recipe.id]: true }));
    await onAdd(recipe, servingsFor(recipe));
    setAdding(prev => ({ ...prev, [recipe.id]: false }));
  };

  return (
    <section className="plan-panel" aria-labelledby="add-recipes-title">
      <h2 id="add-recipes-title">Add from library</h2>
      <input
        type="search"
        className="plan-panel-search"
        placeholder="Search by dish or ingredient"
        aria-label="Search recipes"
        value={filter}
        onChange={e => setFilter(e.target.value)}
      />
      <div className="plan-recipe-browser">
        {available.map(recipe => (
          <BrowseRecipeCard
            key={recipe.id}
            recipe={recipe}
            servings={servingsFor(recipe)}
            onServingsChange={v => setServings(prev => ({ ...prev, [recipe.id]: v }))}
            onAdd={() => add(recipe)}
            adding={!!adding[recipe.id]}
            inventory={inventory}
          />
        ))}
        {available.length === 0 && !browse.loading && (
          <p className="empty-state">{filter ? 'No recipes match.' : 'No recipes to add.'}</p>
        )}
      </div>
    </section>
  );
}
