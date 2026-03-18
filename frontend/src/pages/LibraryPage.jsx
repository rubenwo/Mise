import { useState } from 'react';
import RecipeList from '../components/RecipeList';
import { useRecipes } from '../hooks/useRecipes';

export default function LibraryPage() {
  const { recipes, total, loading, error, search, remove } = useRecipes();
  const [query, setQuery] = useState('');

  const handleSearch = (e) => {
    e.preventDefault();
    if (query.trim()) {
      search({ query: query.trim() });
    }
  };

  return (
    <div className="library-page">
      <h2>Recipe Library</h2>
      <form className="search-bar" onSubmit={handleSearch}>
        <input
          type="text"
          placeholder="Search recipes..."
          value={query}
          onChange={e => setQuery(e.target.value)}
        />
        <button type="submit" className="btn btn-secondary">Search</button>
      </form>
      {total > 0 && <p className="total-count">{total} recipe{total !== 1 ? 's' : ''}</p>}
      {error && <div className="error-message">{error}</div>}
      {loading ? <p>Loading...</p> : <RecipeList recipes={recipes} onDelete={remove} />}
    </div>
  );
}
