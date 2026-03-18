import { useState } from 'react';
import RecipeList from '../components/RecipeList';
import { useRecipes } from '../hooks/useRecipes';
import { filterRecipes } from '../utils/fuzzyMatch';

export default function LibraryPage() {
  const { recipes, total, loading, error, remove } = useRecipes();
  const [query, setQuery] = useState('');

  const filtered = filterRecipes(query, recipes);

  return (
    <div className="library-page">
      <h2>Recipe Library</h2>
      <div className="search-bar">
        <input
          type="text"
          placeholder="Search recipes..."
          value={query}
          onChange={e => setQuery(e.target.value)}
        />
      </div>
      {total > 0 && <p className="total-count">{query ? `${filtered.length} of ${total}` : total} recipe{(query ? filtered.length : total) !== 1 ? 's' : ''}</p>}
      {error && <div className="error-message">{error}</div>}
      {loading ? <p>Loading...</p> : <RecipeList recipes={filtered} onDelete={remove} />}
    </div>
  );
}
