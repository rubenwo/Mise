import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Search, Sparkles } from 'lucide-react';
import DuplicatesModal from '../components/DuplicatesModal';
import LibraryCard from '../components/LibraryCard';
import LibraryRecommendChat from '../components/LibraryRecommendChat';
import Menu from '../components/ui/Menu';
import {
  findDuplicates, getFavourites, getRecommendations, librarySearch, listCuisines, listRecipes, recordImpressions,
} from '../api/client';
import { useEatCounts } from '../hooks/useEatCounts';
import { errorMessage, useFetch } from '../hooks/useFetch';
import type { Recipe } from '../types';

type View = 'new' | 'favourites' | 'all';

const VIEWS: { id: View; label: string; summary: string }[] = [
  { id: 'new', label: 'New to you', summary: 'Dishes you haven’t cooked, spread across flavour families' },
  { id: 'favourites', label: 'Favourites', summary: 'Rated 7 or higher, best first' },
  { id: 'all', label: 'Everything', summary: 'Every recipe, newest first' },
];
const PAGE = 24;

interface Item {
  recipe: Recipe;
  reason?: string;
}

export default function LibraryPage() {
  const [params, setParams] = useSearchParams();
  const view = (params.get('view') as View) || 'new';
  const cuisine = params.get('cuisine') ?? '';
  const quick = params.get('quick') === '1';
  const veg = params.get('veg') === '1';
  const [query, setQuery] = useState('');
  const [debounced, setDebounced] = useState('');
  const [pages, setPages] = useState(1);
  const [asking, setAsking] = useState(false);
  const [dedup, setDedup] = useState<null | 'loading' | Recipe[][]>(null);
  const eatCounts = useEatCounts();
  const cuisines = useFetch('cuisines', listCuisines);
  const total = (cuisines.data ?? []).reduce((s, c) => s + c.count, 0);

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);

  const setParam = (key: string, value: string) => {
    setPages(1);
    setParams(prev => {
      const next = new URLSearchParams(prev);
      if (value) next.set(key, value); else next.delete(key);
      return next;
    }, { replace: true });
  };

  const filters = { cuisine, max_minutes: quick ? 30 : 0, dietary: veg ? 'vegetarian' : '' };
  const searching = debounced !== '';
  const filtered = Boolean(cuisine || quick || veg);
  const key = JSON.stringify({ view, filters, pages, debounced });

  const results = useFetch<{ items: Item[]; more: boolean }>(`library:${key}`, async signal => {
    if (searching || (view === 'all' && filtered)) {
      const res = await librarySearch({
        keywords: debounced,
        cuisine_type: cuisine || undefined,
        max_total_minutes: quick ? 30 : undefined,
        dietary_restrictions: veg ? ['vegetarian'] : undefined,
        limit: PAGE * pages,
      }, signal);
      return { items: res.recipes.map(recipe => ({ recipe })), more: res.recipes.length === PAGE * pages };
    }
    if (view === 'new') {
      const picks = await getRecommendations({ ...filters, limit: Math.min(96, PAGE * pages) }, signal);
      return { items: picks.map(p => ({ recipe: p.recipe, reason: p.reason })), more: picks.length === PAGE * pages && pages < 4 };
    }
    if (view === 'favourites') {
      const favs = (await getFavourites(signal)).filter(r =>
        (!cuisine || r.cuisine_type === cuisine) &&
        (!quick || (r.prep_time_minutes + r.cook_time_minutes > 0 && r.prep_time_minutes + r.cook_time_minutes <= 30)) &&
        (!veg || r.dietary_restrictions?.some(d => d.toLowerCase() === 'vegetarian')));
      return { items: favs.map(recipe => ({ recipe })), more: false };
    }
    const res = await listRecipes(PAGE * pages, 0, cuisine, signal);
    return { items: (res.recipes ?? []).map(recipe => ({ recipe })), more: (res.recipes ?? []).length < res.total };
  });
  const items = results.data?.items ?? [];

  // Note the first screenful of discovery picks as seen, so ones you keep
  // passing over fade out of future recommendations.
  const firstPageIds = view === 'new' && !searching && pages === 1 ? items.slice(0, 12).map(i => i.recipe.id).join(',') : '';
  useEffect(() => {
    if (firstPageIds) recordImpressions(firstPageIds.split(',').map(Number)).catch(() => {});
  }, [firstPageIds]);

  const findDupes = () => {
    setDedup('loading');
    findDuplicates()
      .then(data => setDedup(data.groups ?? []))
      .catch(err => { alert('Failed to find duplicates: ' + errorMessage(err)); setDedup(null); });
  };

  const summary = searching
    ? `${items.length}${results.data?.more ? '+' : ''} recipes match “${debounced}”`
    : VIEWS.find(v => v.id === view)?.summary;

  return (
    <div className="library-page lib">
      <div className="page-header">
        <h1>Library</h1>
        <div className="lib-header-side">
          {total > 0 && <span className="lib-total">{total} recipes</span>}
          <Menu label="Library actions" items={[
            { label: dedup === 'loading' ? 'Scanning for duplicates…' : 'Find duplicates', onSelect: findDupes },
          ]} />
        </div>
      </div>
      {Array.isArray(dedup) && (
        <DuplicatesModal
          groups={dedup}
          onClose={() => setDedup(null)}
          onDeleted={ids => {
            const gone = new Set(ids);
            setDedup(prev => Array.isArray(prev) ? prev.map(g => g.filter(r => !gone.has(r.id))).filter(g => g.length >= 2) : null);
            cuisines.reload();
            results.reload();
          }}
        />
      )}

      <div className="lib-search">
        <label className="lib-search-box">
          <Search size={18} aria-hidden="true" />
          <span className="visually-hidden">Search recipes</span>
          <input type="search" placeholder="Search by dish or ingredient" value={query} onChange={e => { setQuery(e.target.value); setPages(1); }} />
        </label>
        <button type="button" className="btn btn-secondary lib-ask" aria-pressed={asking} onClick={() => setAsking(a => !a)}>
          <Sparkles size={17} aria-hidden="true" />Ask for ideas
        </button>
      </div>

      {asking ? (
        <LibraryRecommendChat limit={6} onDeleteRecipe={() => results.reload()} />
      ) : (
        <>
          <div className="lib-filters" role="group" aria-label="Filters">
            {!searching && VIEWS.map(v => (
              <button key={v.id} type="button" className="filter-chip" aria-pressed={view === v.id}
                onClick={() => setParam('view', v.id === 'new' ? '' : v.id)}>
                {v.label}
              </button>
            ))}
            {!searching && <span className="lib-filters-divider" aria-hidden="true" />}
            <button type="button" className="filter-chip" aria-pressed={quick} onClick={() => setParam('quick', quick ? '' : '1')}>Under 30 min</button>
            <button type="button" className="filter-chip" aria-pressed={veg} onClick={() => setParam('veg', veg ? '' : '1')}>Vegetarian</button>
            <label className="visually-hidden" htmlFor="lib-cuisine">Cuisine</label>
            <select id="lib-cuisine" className="filter-select" value={cuisine} onChange={e => setParam('cuisine', e.target.value)}>
              <option value="">All cuisines</option>
              {(cuisines.data ?? []).map(c => (
                <option key={c.cuisine_type} value={c.cuisine_type}>{c.cuisine_type} ({c.count})</option>
              ))}
            </select>
          </div>

          {summary && <p className="lib-summary">{summary}</p>}
          {results.error && <div className="error-message">{results.error}</div>}

          {results.loading && !results.data ? (
            <p className="empty-state">Loading…</p>
          ) : items.length === 0 ? (
            <p className="empty-state">
              {view === 'favourites' && !searching ? 'Nothing rated 7 or higher matches yet.' : 'No recipes match.'}
            </p>
          ) : (
            <div className="lib-grid">
              {items.map(({ recipe, reason }) => (
                <LibraryCard
                  key={recipe.id}
                  recipe={recipe}
                  reason={reason}
                  stats={eatCounts[recipe.id]}
                  detail={view === 'favourites' && !searching ? 'rating' : 'description'}
                />
              ))}
            </div>
          )}

          {results.data?.more && (
            <button type="button" className="btn btn-secondary lib-more" disabled={results.loading} onClick={() => setPages(p => p + 1)}>
              {results.loading ? 'Loading…' : 'Show more'}
            </button>
          )}
        </>
      )}
    </div>
  );
}
