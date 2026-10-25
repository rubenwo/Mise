import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { createPlan, deletePlan, listPlans } from '../../api/client';
import Menu from '../../components/ui/Menu';
import { errorMessage, useFetch } from '../../hooks/useFetch';
import { defaultPlanName, planStatusLabel } from '../../utils/format';
import type { MealPlan } from '../../types';

function PlanCard({ plan, onDelete }: { plan: MealPlan; onDelete: () => void }) {
  const images = plan.preview_images ?? [];
  const count = plan.recipe_count ?? 0;
  const cooked = plan.cooked_count ?? 0;
  return (
    <article className="plan-card">
      <Link to={`/plans/${plan.id}`} className="plan-card-link">
        <div className={`plan-card-mosaic plan-card-mosaic-${Math.min(images.length, 4)}`} aria-hidden="true">
          {images.map(src => <img key={src} src={src} alt="" loading="lazy" />)}
        </div>
        <div className="plan-card-body">
          <span className="plan-card-name">{plan.name}</span>
          <span className="plan-card-meta">
            {count === 0 ? 'No dinners yet' : `${count} ${count === 1 ? 'dinner' : 'dinners'} · ${cooked} cooked`}
          </span>
        </div>
      </Link>
      <div className="plan-card-footer">
        <span className={`plan-status plan-status-${plan.status}`}>{planStatusLabel(plan.status)}</span>
        <Menu label={`Actions for ${plan.name}`} items={[{ label: 'Delete plan', danger: true, onSelect: onDelete }]} />
      </div>
    </article>
  );
}

export default function PlansList() {
  const { data: plans, loading, error, setData: setPlans } = useFetch('plans', listPlans);
  const [name, setName] = useState(defaultPlanName);
  const navigate = useNavigate();

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    try {
      const plan = await createPlan(name.trim());
      navigate(`/plans/${plan.id}`);
    } catch (err) {
      alert('Failed to create plan: ' + errorMessage(err));
    }
  };

  const handleDelete = async (plan: MealPlan) => {
    if (!confirm(`Delete "${plan.name}"? Cook history and ratings from this plan are deleted too.`)) return;
    try {
      await deletePlan(plan.id);
      setPlans(prev => prev?.filter(p => p.id !== plan.id));
    } catch (err) {
      alert('Failed to delete plan: ' + errorMessage(err));
    }
  };

  return (
    <div className="plans-page">
      <div className="page-header">
        <h1>Plans</h1>
      </div>
      <form onSubmit={handleCreate} className="plan-create-form">
        <label className="visually-hidden" htmlFor="plan-name">New plan name</label>
        <input id="plan-name" type="text" value={name} onChange={e => setName(e.target.value)} />
        <button className="btn btn-primary" type="submit" disabled={!name.trim()}>New plan</button>
      </form>

      {error && <div className="error-message">{error}</div>}
      {loading && !plans ? (
        <p className="empty-state">Loading...</p>
      ) : !plans || plans.length === 0 ? (
        <p className="empty-state">No plans yet. Start one above.</p>
      ) : (
        <div className="plan-grid">
          {plans.map(p => <PlanCard key={p.id} plan={p} onDelete={() => handleDelete(p)} />)}
        </div>
      )}
    </div>
  );
}
