import { useState, useLayoutEffect, useRef, type FormEvent, type RefObject } from 'react';
import { listPlans, createPlan, addRecipeToPlan } from '../api/client';
import { errorMessage, useFetch } from '../hooks/useFetch';

interface AddToPlanMenuProps {
  recipeId: number;
  /** The button the menu opens from; read after render to position the menu. */
  anchorRef: RefObject<HTMLElement | null>;
  onClose: () => void;
}

export default function AddToPlanMenu({ recipeId, anchorRef, onClose }: AddToPlanMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const { data: allPlans, loading } = useFetch('plans', listPlans);
  const plans = (allPlans ?? []).filter(p => p.status === 'draft');
  const [adding, setAdding] = useState<number | null>(null);
  const [done, setDone] = useState<number | 'new' | null>(null);
  const [showNewPlan, setShowNewPlan] = useState(false);
  const [newPlanName, setNewPlanName] = useState('');
  const [creating, setCreating] = useState(false);

  // Position the menu near the anchor button before paint.
  useLayoutEffect(() => {
    const anchorEl = anchorRef.current;
    if (!anchorEl || !menuRef.current) return;
    const rect = anchorEl.getBoundingClientRect();
    const menu = menuRef.current;
    const menuWidth = 240;
    let left = rect.left;
    let top = rect.bottom + 6;

    // Keep within viewport
    if (left + menuWidth > window.innerWidth - 8) {
      left = Math.max(8, rect.right - menuWidth);
    }
    if (top + 300 > window.innerHeight) {
      top = Math.max(8, rect.top - 300 - 6);
    }

    menu.style.left = `${left}px`;
    menu.style.top = `${top}px`;
  }, [anchorRef]);

  const handleAddTo = async (planId: number) => {
    setAdding(planId);
    try {
      await addRecipeToPlan(planId, recipeId, 4);
      setDone(planId);
      setTimeout(onClose, 900);
    } catch (err) {
      alert('Could not add to plan: ' + errorMessage(err));
      setAdding(null);
    }
  };

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    if (!newPlanName.trim()) return;
    setCreating(true);
    try {
      const plan = await createPlan(newPlanName.trim());
      await addRecipeToPlan(plan.id, recipeId, 4);
      setDone('new');
      setTimeout(onClose, 900);
    } catch (err) {
      alert('Could not create plan: ' + errorMessage(err));
      setCreating(false);
    }
  };

  return (
    <>
      <div className="add-to-plan-backdrop" onClick={onClose} />
      <div className="add-to-plan-menu" ref={menuRef}>
        <div className="add-to-plan-header">Add to plan</div>

        {loading ? (
          <div className="add-to-plan-empty">Loading...</div>
        ) : (
          <>
            {plans.length === 0 && !showNewPlan && (
              <div className="add-to-plan-empty">No draft plans yet</div>
            )}
            {plans.map(plan => (
              <button
                key={plan.id}
                className={`add-to-plan-item${done === plan.id ? ' add-to-plan-item-done' : ''}`}
                onClick={() => handleAddTo(plan.id)}
                disabled={adding !== null || done !== null}
              >
                <span>{plan.name}</span>
                {adding === plan.id && <span className="add-to-plan-spinner" />}
                {done === plan.id && <span className="add-to-plan-check">&#10003;</span>}
              </button>
            ))}

            {showNewPlan ? (
              <form className="add-to-plan-form" onSubmit={handleCreate}>
                <input
                  autoFocus
                  type="text"
                  placeholder="Plan name…"
                  value={newPlanName}
                  onChange={e => setNewPlanName(e.target.value)}
                />
                <button type="submit" disabled={!newPlanName.trim() || creating}>
                  {creating ? '…' : 'Create & add'}
                </button>
              </form>
            ) : (
              <button className="add-to-plan-new" onClick={() => setShowNewPlan(true)}>
                + New plan
              </button>
            )}
          </>
        )}
      </div>
    </>
  );
}
