import { useEffect, useState, useCallback } from 'react';
import RecipeCard from '../components/RecipeCard';
import RecipeList from '../components/RecipeList';
import DuplicatesModal from '../components/DuplicatesModal';
import { aiSearchRecipes, findDuplicates, getRecipeSuggestions, getSettings, listCuisines, listRecipes, librarySearch, deleteRecipe } from '../api/client';

const CUISINE_COLORS = [
  '#c2410c', '#0d9488', '#7c3aed', '#b45309',
  '#0369a1', '#15803d', '#be123c', '#6d28d9',
  '#0f766e', '#b45309',
];

function cuisineColor(name) {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
  return CUISINE_COLORS[hash % CUISINE_COLORS.length];
}

function CuisineGroup({ cuisine, count, previewImages, expanded, onToggle, onDelete }) {
  const [recipes, setRecipes] = useState([]);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const color = cuisineColor(cuisine);

  useEffect(() => {
    if (!expanded || loaded) return;
    setLoading(true);
    listRecipes(1000, 0, cuisine === 'Other' ? '' : cuisine)
      .then(data => {
        // For "Other", filter to recipes with empty cuisine_type
        const recipes = data.recipes || [];
        setRecipes(cuisine === 'Other' ? recipes.filter(r => !r.cuisine_type) : recipes);
        setLoaded(true);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [expanded, loaded, cuisine]);

  const handleDelete = async (id) => {
    try {
      await deleteRecipe(id);
      setRecipes(prev => prev.filter(r => r.id !== id));
      onDelete(id);
    } catch (err) {
      console.error('Failed to delete recipe', err);
    }
  };

  return (
    <div className={`cuisine-group${expanded ? ' cuisine-group--expanded' : ''}`}>
      <button className="cuisine-group-header" onClick={onToggle} style={{ '--cuisine-color': color }}>
        <div className="cuisine-group-info">
          <span className="cuisine-group-name">{cuisine}</span>
          <span className="cuisine-group-count">{count} recipe{count !== 1 ? 's' : ''}</span>
        </div>
        <div className="cuisine-group-right">
          {!expanded && previewImages.length > 0 && (
            <div className="cuisine-group-previews">
              {previewImages.map((url, i) => (
                <img key={i} className="cuisine-preview-img" src={url} alt="" />
              ))}
            </div>
          )}
          <span className="cuisine-group-arrow">{expanded ? '▲' : '▼'}</span>
        </div>
      </button>
      {expanded && (
        <div className="cuisine-group-body">
          {loading ? (
            <p style={{ padding: '1rem' }}>Loading...</p>
          ) : (
            <div className="recipe-list">
              {recipes.map(recipe => (
                <RecipeCard key={recipe.id} recipe={recipe} showLink onDelete={handleDelete} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function SuggestedCarousel({ count }) {
  const [recipes, setRecipes] = useState([]);

  useEffect(() => {
    if (!count || count < 1) return;
    getRecipeSuggestions(count)
      .then(data => setRecipes(data || []))
      .catch(() => {});
  }, [count]);

  if (recipes.length === 0) return null;

  return (
    <div className="suggestions-carousel">
      <div className="suggestions-carousel-header">
        <h3>Today's picks</h3>
        <span className="suggestions-carousel-hint">Refreshes daily · based on your library</span>
      </div>
      <div className="suggestions-carousel-track">
        {recipes.map(recipe => (
          <RecipeCard key={recipe.id} recipe={recipe} showLink />
        ))}
      </div>
    </div>
  );
}

export default function LibraryPage() {
  const [cuisines, setCuisines] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState('fuzzy'); // 'fuzzy' | 'ai'
  const [searchResults, setSearchResults] = useState(null); // { recipes, total } | null
  const [searchLoading, setSearchLoading] = useState(false);
  const [searchError, setSearchError] = useState(null);
  const [expandedCuisines, setExpandedCuisines] = useState(new Set());
  const [suggestionCount, setSuggestionCount] = useState(3);
  const [dedupState, setDedupState] = useState(null);

  const loadCuisines = useCallback(() => {
    setLoading(true);
    listCuisines()
      .then(data => {
        setCuisines(data || []);
        setTotal((data || []).reduce((sum, c) => sum + c.count, 0));
        setError(null);
      })
      .catch(err => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    loadCuisines();
  }, [loadCuisines]);

  useEffect(() => {
    getSettings()
      .then(data => {
        const map = {};
        (data || []).forEach(s => { map[s.key] = s.value; });
        if (map.suggestion_count) setSuggestionCount(parseInt(map.suggestion_count, 10) || 3);
      })
      .catch(() => {});
  }, []);

  const runSearch = useCallback((q) => {
    if (!q.trim()) {
      setSearchResults(null);
      setSearchError(null);
      return;
    }
    setSearchLoading(true);
    setSearchError(null);
    librarySearch({ keywords: q.trim(), limit: 200 })
      .then(data => setSearchResults(data))
      .catch(err => setSearchError(err.message))
      .finally(() => setSearchLoading(false));
  }, []);

  // Debounced fuzzy search against backend
  useEffect(() => {
    if (searchMode !== 'fuzzy') return;
    const timer = setTimeout(() => runSearch(query), 300);
    return () => clearTimeout(timer);
  }, [query, searchMode, runSearch]);

  const handleAiSearch = () => {
    if (!query.trim()) {
      setSearchResults(null);
      setSearchError(null);
      return;
    }
    setSearchLoading(true);
    setSearchError(null);
    aiSearchRecipes(query.trim())
      .then(data => setSearchResults(data))
      .catch(err => setSearchError(err.message))
      .finally(() => setSearchLoading(false));
  };

  const handleModeChange = (mode) => {
    setSearchMode(mode);
    setSearchResults(null);
    setSearchError(null);
    setSearchLoading(false);
  };

  const handleFindDuplicates = () => {
    setDedupState('loading');
    findDuplicates()
      .then(data => setDedupState({ groups: data.groups || [] }))
      .catch(err => { alert('Failed to find duplicates: ' + err.message); setDedupState(null); });
  };

  const handleDedupDeleted = (ids) => {
    loadCuisines();
    setDedupState(prev => {
      if (!prev || !prev.groups) return null;
      const idSet = new Set(ids);
      const groups = prev.groups
        .map(g => g.filter(r => !idSet.has(r.id)))
        .filter(g => g.length >= 2);
      return { groups };
    });
  };

  const handleDeleteFromCuisine = () => {
    // Refresh cuisine counts when a recipe is deleted from an expanded group
    listCuisines()
      .then(data => {
        setCuisines(data || []);
        setTotal((data || []).reduce((sum, c) => sum + c.count, 0));
      })
      .catch(() => {});
  };

  const toggleCuisine = (cuisine) => {
    setExpandedCuisines(prev => {
      const next = new Set(prev);
      next.has(cuisine) ? next.delete(cuisine) : next.add(cuisine);
      return next;
    });
  };

  const isSearching = query.trim().length > 0;
  const displayCount = isSearching ? (searchResults ? searchResults.total : 0) : total;

  return (
    <div className="library-page">
      <div className="library-page-header">
        <h2>Recipe Library</h2>
        <button
          className="btn btn-secondary"
          onClick={handleFindDuplicates}
          disabled={dedupState === 'loading' || total === 0}
        >
          {dedupState === 'loading' ? 'Scanning…' : 'Find Duplicates'}
        </button>
      </div>
      {dedupState && dedupState !== 'loading' && (
        <DuplicatesModal
          groups={dedupState.groups}
          onClose={() => setDedupState(null)}
          onDeleted={handleDedupDeleted}
        />
      )}
      <div className="search-bar">
        <input
          type="text"
          placeholder={searchMode === 'ai' ? 'Describe what you\'re looking for...' : 'Search recipes...'}
          value={query}
          onChange={e => setQuery(e.target.value)}
          onKeyDown={e => { if (searchMode === 'ai' && e.key === 'Enter') handleAiSearch(); }}
        />
      </div>
      <div className="mode-toggle">
        <button
          className={searchMode === 'fuzzy' ? 'active' : ''}
          onClick={() => handleModeChange('fuzzy')}
        >
          Keyword
        </button>
        <button
          className={searchMode === 'ai' ? 'active' : ''}
          onClick={() => handleModeChange('ai')}
        >
          AI Search
        </button>
      </div>
      {total > 0 && (
        <p className="total-count">
          {isSearching
            ? `${displayCount} of ${total}`
            : total} recipe{displayCount !== 1 ? 's' : ''}
          {searchLoading && ' · searching...'}
        </p>
      )}
      {(error || searchError) && <div className="error-message">{error || searchError}</div>}
      {searchMode === 'ai' && searchResults?.interpreted && isSearching && (
        <p className="total-count" style={{ marginBottom: 12 }}>
          {[
            searchResults.interpreted.query && `"${searchResults.interpreted.query}"`,
            searchResults.interpreted.cuisine_type && searchResults.interpreted.cuisine_type,
            ...(searchResults.interpreted.dietary_restrictions || []),
            ...(searchResults.interpreted.tags || []),
            searchResults.interpreted.max_total_minutes > 0 && `≤${searchResults.interpreted.max_total_minutes} min`,
          ].filter(Boolean).join(' · ')}
        </p>
      )}
      {!isSearching && !loading && total >= suggestionCount && (
        <SuggestedCarousel count={suggestionCount} />
      )}
      {loading ? (
        <p>Loading...</p>
      ) : isSearching ? (
        searchLoading ? null : (
          <RecipeList
            recipes={searchResults?.recipes || []}
            onDelete={() => { loadCuisines(); setSearchResults(null); setQuery(''); }}
            searchQuery={query}
          />
        )
      ) : (
        <div className="cuisine-groups">
          {cuisines.map(c => (
            <CuisineGroup
              key={c.cuisine_type}
              cuisine={c.cuisine_type}
              count={c.count}
              previewImages={c.preview_images || []}
              expanded={expandedCuisines.has(c.cuisine_type)}
              onToggle={() => toggleCuisine(c.cuisine_type)}
              onDelete={handleDeleteFromCuisine}
            />
          ))}
        </div>
      )}
    </div>
  );
}
