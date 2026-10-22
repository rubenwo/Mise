import { useEffect, useState, type CSSProperties } from 'react';
import RecipeCard from '../components/RecipeCard';
import RecipeList from '../components/RecipeList';
import DuplicatesModal from '../components/DuplicatesModal';
import LibraryRecommendChat from '../components/LibraryRecommendChat';
import { findDuplicates, getRecipeSuggestions, getSettings, listCuisines, listRecipes, librarySearch, deleteRecipe } from '../api/client';
import { errorMessage, useFetch } from '../hooks/useFetch';
import type { Recipe } from '../types';

const CUISINE_COLORS = [
  '#c2410c', '#0d9488', '#7c3aed', '#b45309',
  '#0369a1', '#15803d', '#be123c', '#6d28d9',
  '#0f766e', '#b45309',
];

function cuisineColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
  return CUISINE_COLORS[hash % CUISINE_COLORS.length];
}

interface CuisineGroupProps {
  cuisine: string;
  count: number;
  previewImages: string[];
  expanded: boolean;
  onToggle: () => void;
  onDelete: (id: number) => Promise<void>;
}

function CuisineGroup({ cuisine, count, previewImages, expanded, onToggle, onDelete }: CuisineGroupProps) {
  // Load lazily on first expand, then keep the list when collapsed again.
  const [everExpanded, setEverExpanded] = useState(expanded);
  if (expanded && !everExpanded) setEverExpanded(true);

  const { data: recipes, loading, setData } = useFetch(
    everExpanded ? `cuisine:${cuisine}` : null,
    async signal => {
      const data = await listRecipes(1000, 0, cuisine === 'Other' ? '' : cuisine, signal);
      const all = data.recipes ?? [];
      // "Other" groups recipes with an empty cuisine_type.
      return cuisine === 'Other' ? all.filter(r => !r.cuisine_type) : all;
    },
  );
  const color = cuisineColor(cuisine);

  const handleDelete = (id: number) => {
    onDelete(id)
      .then(() => setData(prev => prev?.filter(r => r.id !== id)))
      .catch(() => {}); // already reported by onDelete
  };

  return (
    <div className={`cuisine-group${expanded ? ' cuisine-group--expanded' : ''}`}>
      <button className="cuisine-group-header" onClick={onToggle} style={{ '--cuisine-color': color } as CSSProperties}>
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
          {loading && !recipes ? (
            <p style={{ padding: '1rem' }}>Loading...</p>
          ) : (
            <div className="recipe-list">
              {(recipes ?? []).map(recipe => (
                <RecipeCard key={recipe.id} recipe={recipe} showLink onDelete={handleDelete} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function SuggestedCarousel({ count }: { count: number }) {
  const { data: recipes } = useFetch(
    count >= 1 ? `suggestions:${count}` : null,
    signal => getRecipeSuggestions(count, signal),
  );

  if (!recipes || recipes.length === 0) return null;

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

type SearchMode = 'fuzzy' | 'recommend';
type DedupState = null | 'loading' | { groups: Recipe[][] };

export default function LibraryPage() {
  const cuisinesQuery = useFetch('cuisines', listCuisines);
  const cuisines = cuisinesQuery.data ?? [];
  const total = cuisines.reduce((sum, c) => sum + c.count, 0);
  const reloadCuisines = cuisinesQuery.reload;

  const settings = useFetch('settings', getSettings);
  const suggestionSetting = settings.data?.find(s => s.key === 'suggestion_count')?.value;
  const suggestionCount = (suggestionSetting && parseInt(suggestionSetting, 10)) || 3;

  const [query, setQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('fuzzy');
  const [expandedCuisines, setExpandedCuisines] = useState<Set<string>>(new Set());
  const [dedupState, setDedupState] = useState<DedupState>(null);

  // Debounce keyword search against the backend.
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);

  const search = useFetch(
    searchMode === 'fuzzy' && debouncedQuery ? `search:${debouncedQuery}` : null,
    signal => librarySearch({ keywords: debouncedQuery, limit: 200 }, signal),
  );

  const handleModeChange = (mode: SearchMode) => {
    setSearchMode(mode);
    if (mode !== 'fuzzy') setQuery('');
  };

  const handleDeleteRecipe = async (id: number) => {
    try {
      await deleteRecipe(id);
      reloadCuisines();
      search.setData(prev => prev && { recipes: prev.recipes.filter(r => r.id !== id), total: prev.total - 1 });
    } catch (err) {
      alert('Failed to delete recipe: ' + errorMessage(err));
      throw err;
    }
  };

  const handleFindDuplicates = () => {
    setDedupState('loading');
    findDuplicates()
      .then(data => setDedupState({ groups: data.groups ?? [] }))
      .catch(err => { alert('Failed to find duplicates: ' + errorMessage(err)); setDedupState(null); });
  };

  const handleDedupDeleted = (ids: number[]) => {
    reloadCuisines();
    setDedupState(prev => {
      if (!prev || prev === 'loading') return null;
      const idSet = new Set(ids);
      const groups = prev.groups
        .map(g => g.filter(r => !idSet.has(r.id)))
        .filter(g => g.length >= 2);
      return { groups };
    });
  };

  const toggleCuisine = (cuisine: string) => {
    setExpandedCuisines(prev => {
      const next = new Set(prev);
      if (next.has(cuisine)) next.delete(cuisine);
      else next.add(cuisine);
      return next;
    });
  };

  const isRecommend = searchMode === 'recommend';
  const isSearching = !isRecommend && query.trim().length > 0;
  // Results are pending until the debounce settles and the request returns.
  const searchPending = isSearching && (query.trim() !== debouncedQuery || search.loading);
  const displayCount = isSearching ? (search.data?.total ?? 0) : total;
  const error = cuisinesQuery.error ?? (isSearching ? search.error : undefined);

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
      <div className="mode-toggle">
        <button
          className={searchMode === 'fuzzy' ? 'active' : ''}
          onClick={() => handleModeChange('fuzzy')}
        >
          Keyword
        </button>
        <button
          className={searchMode === 'recommend' ? 'active' : ''}
          onClick={() => handleModeChange('recommend')}
        >
          Recommend
        </button>
      </div>
      {!isRecommend && (
        <div className="search-bar">
          <input
            type="text"
            placeholder="Search recipes..."
            value={query}
            onChange={e => setQuery(e.target.value)}
          />
        </div>
      )}
      {total > 0 && !isRecommend && (
        <p className="total-count">
          {isSearching
            ? `${displayCount} of ${total}`
            : total} recipe{displayCount !== 1 ? 's' : ''}
          {searchPending && ' · searching...'}
        </p>
      )}
      {error && <div className="error-message">{error}</div>}
      {!isSearching && !isRecommend && !cuisinesQuery.loading && total >= suggestionCount && (
        <SuggestedCarousel count={suggestionCount} />
      )}
      {isRecommend ? (
        <LibraryRecommendChat
          limit={suggestionCount}
          onDeleteRecipe={id => { handleDeleteRecipe(id).catch(() => {}); }}
        />
      ) : cuisinesQuery.loading && !cuisinesQuery.data ? (
        <p>Loading...</p>
      ) : isSearching ? (
        searchPending ? null : (
          <RecipeList
            recipes={search.data?.recipes ?? []}
            onDelete={id => { handleDeleteRecipe(id).catch(() => {}); }}
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
              previewImages={c.preview_images ?? []}
              expanded={expandedCuisines.has(c.cuisine_type)}
              onToggle={() => toggleCuisine(c.cuisine_type)}
              onDelete={handleDeleteRecipe}
            />
          ))}
        </div>
      )}
    </div>
  );
}
