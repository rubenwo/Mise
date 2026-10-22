import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { createPlan, deletePlan, listPlans } from '../../api/client';
import { errorMessage, useFetch } from '../../hooks/useFetch';
import { planStatusLabel } from '../../utils/format';

export default function PlansList() {
  const { data: plans, loading, error, setData: setPlans } = useFetch('plans', listPlans);
  const [name, setName] = useState('');
  const navigate = useNavigate();

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    try {
      const plan = await createPlan(name.trim());
      setName('');
      navigate(`/plans/${plan.id}`);
    } catch (err) {
      alert('Failed to create plan: ' + errorMessage(err));
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await deletePlan(id);
      setPlans(prev => prev?.filter(p => p.id !== id));
    } catch (err) {
      alert('Failed to delete plan: ' + errorMessage(err));
    }
  };

  return (
    <div className="plans-page">
      <h2>Meal Plans</h2>
      <form onSubmit={handleCreate} className="plan-create-form">
        <input
          type="text"
          placeholder="New plan name (e.g. Week 12)"
          value={name}
          onChange={e => setName(e.target.value)}
        />
        <button className="btn btn-primary" type="submit" disabled={!name.trim()}>Create Plan</button>
      </form>

      {error && <div className="error-message">{error}</div>}
      {loading && !plans ? (
        <p className="empty-state">Loading...</p>
      ) : !plans || plans.length === 0 ? (
        <p className="empty-state">No meal plans yet. Create one above.</p>
      ) : (
        <div className="plans-list">
          {plans.map(p => (
            <div key={p.id} className="plan-list-item">
              <div className="plan-list-info">
                <Link to={`/plans/${p.id}`} className="plan-list-name">{p.name}</Link>
                <span className={`plan-status plan-status-${p.status}`}>{planStatusLabel(p.status)}</span>
              </div>
              <button className="btn btn-danger" onClick={() => handleDelete(p.id)}>Delete</button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
