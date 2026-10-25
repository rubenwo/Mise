import { useState } from 'react';
import { getGroceryChecks, getLastAHSearch, setGroceryCheck } from '../../api/client';
import AHOrderModal from '../../components/AHOrderModal';
import { errorMessage, useFetch } from '../../hooks/useFetch';
import { formatDate } from '../../utils/format';
import { isInStock } from '../../utils/inventoryMatch';
import type { AggregatedIngredient, InventoryItem } from '../../types';
import LeftoversModal from './LeftoversModal';

interface GroceryPanelProps {
  planId: number;
  ingredients: AggregatedIngredient[];
  loading: boolean;
  inventory: InventoryItem[];
}

const amountText = (i: AggregatedIngredient) => [i.amount || '', i.unit].filter(Boolean).join(' ');

export default function GroceryPanel({ planId, ingredients, loading, inventory }: GroceryPanelProps) {
  const [tab, setTab] = useState<'buy' | 'have'>('buy');
  const [showAH, setShowAH] = useState(false);
  const [showLeftovers, setShowLeftovers] = useState(false);
  const [copied, setCopied] = useState(false);
  const checks = useFetch(`grocery-checks:${planId}`, signal => getGroceryChecks(planId, signal));
  const ah = useFetch(`ah-search:${planId}`, signal => getLastAHSearch(planId, signal));
  const checked = new Set(checks.data ?? []);

  const sorted = [...ingredients].sort((a, b) => a.name.localeCompare(b.name));
  const have = sorted.filter(i => isInStock(i.name, inventory));
  const buy = sorted.filter(i => !isInStock(i.name, inventory));
  const shown = tab === 'buy' ? buy : have;

  const toggle = async (name: string) => {
    const next = !checked.has(name);
    const apply = (on: boolean) => checks.setData(prev => {
      const s = new Set(prev ?? []);
      if (on) s.add(name); else s.delete(name);
      return [...s];
    });
    apply(next);
    try {
      await setGroceryCheck(planId, name, next);
    } catch (err) {
      apply(!next);
      alert('Could not save: ' + errorMessage(err));
    }
  };

  const copyList = async () => {
    const text = buy.filter(i => !checked.has(i.name)).map(i => `${amountText(i)} ${i.name}`.trim()).join('\n');
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      alert('Copying is not available here.');
    }
  };

  const search = ah.data;
  const total = search ? search.matched.length + search.not_found.length : 0;

  return (
    <aside className="grocery-panel" aria-labelledby="grocery-title">
      <div className="grocery-head">
        <div className="grocery-title-row">
          <h2 id="grocery-title">Groceries</h2>
          <button type="button" className="btn-link" onClick={() => setShowLeftovers(true)} disabled={ingredients.length === 0}>
            Use up leftovers
          </button>
        </div>
        <div className="segmented" role="tablist" aria-label="Grocery list">
          <button type="button" role="tab" aria-selected={tab === 'buy'} onClick={() => setTab('buy')}>To buy · {buy.length}</button>
          <button type="button" role="tab" aria-selected={tab === 'have'} onClick={() => setTab('have')}>In pantry · {have.length}</button>
        </div>
      </div>

      <div className="grocery-list">
        {loading ? (
          <p className="grocery-empty">Adding up ingredients…</p>
        ) : shown.length === 0 ? (
          <p className="grocery-empty">
            {ingredients.length === 0 ? 'Add dinners to build the list.' : tab === 'buy' ? 'Everything is in your pantry.' : 'Nothing from your pantry is needed.'}
          </p>
        ) : (
          shown.map(i => (
            <label key={i.name} className={`grocery-item${checked.has(i.name) ? ' grocery-item-done' : ''}`} title={i.recipes.join(', ')}>
              <input type="checkbox" checked={checked.has(i.name)} onChange={() => toggle(i.name)} />
              <span className="grocery-item-name">{i.name}</span>
              <span className="grocery-item-amount">{amountText(i)}</span>
            </label>
          ))
        )}
      </div>

      {search && search.not_found.length > 0 && (
        <div className="grocery-note">
          <strong>Not at Albert Heijn</strong>
          <span>{search.not_found.map(i => i.name).join(', ')}. Get these elsewhere or swap them.</span>
        </div>
      )}

      <div className="grocery-foot">
        {search?.searched_at && (
          <p>Searched Albert Heijn {formatDate(search.searched_at)} · {search.matched.length} of {total} found</p>
        )}
        <div className="grocery-actions">
          <button type="button" className="btn btn-primary" onClick={() => setShowAH(true)} disabled={ingredients.length === 0}>
            {search ? 'Search Albert Heijn again' : 'Find at Albert Heijn'}
          </button>
          <button type="button" className="btn btn-secondary" onClick={copyList} disabled={buy.length === 0}>
            {copied ? 'Copied' : 'Copy list'}
          </button>
        </div>
      </div>

      {showAH && <AHOrderModal planId={planId} onClose={() => setShowAH(false)} onResult={ah.setData} />}
      {showLeftovers && <LeftoversModal planId={planId} ingredients={sorted} onClose={() => setShowLeftovers(false)} />}
    </aside>
  );
}
