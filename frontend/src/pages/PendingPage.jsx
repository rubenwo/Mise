import { useState, useEffect, useRef } from 'react';
import { listPendingRecipes, approvePendingRecipe, rejectPendingRecipe } from '../api/client';
import RecipeCard from '../components/RecipeCard';

export default function PendingPage({ onCountChange }) {
  const [recipes, setRecipes] = useState([]);
  const [loading, setLoading] = useState(true);
  const [acting, setActing] = useState({});
  const [generatingMsg, setGeneratingMsg] = useState(null);
  const generatingTimerRef = useRef(null);

  useEffect(() => {
    listPendingRecipes()
      .then(data => {
        const list = data || [];
        setRecipes(list);
        onCountChange?.(list.length);
      })
      .finally(() => setLoading(false));
  }, []);

  // Stream background generation events from the server.
  useEffect(() => {
    const es = new EventSource('/api/pending/events');

    es.onmessage = (e) => {
      let ev;
      try { ev = JSON.parse(e.data); } catch { return; }

      if (ev.type === 'pending_added') {
        const recipe = ev.data;
        setRecipes(rs => {
          // Avoid duplicates if the page somehow receives the same event twice.
          if (rs.some(r => r.id === recipe.id)) return rs;
          const next = [recipe, ...rs];
          onCountChange?.(next.length);
          return next;
        });
        setGeneratingMsg(null);
        clearTimeout(generatingTimerRef.current);
      } else if (ev.type === 'status' || ev.type === 'tool') {
        setGeneratingMsg(ev.message || 'Generating recipe…');
        clearTimeout(generatingTimerRef.current);
        // Clear the banner if nothing new arrives within 60 seconds.
        generatingTimerRef.current = setTimeout(() => setGeneratingMsg(null), 60_000);
      }
    };

    return () => {
      es.close();
      clearTimeout(generatingTimerRef.current);
    };
  }, []);

  const remove = (id) => {
    setRecipes(rs => {
      const next = rs.filter(r => r.id !== id);
      onCountChange?.(next.length);
      return next;
    });
  };

  const handleApprove = async (id) => {
    setActing(a => ({ ...a, [id]: 'approving' }));
    try {
      await approvePendingRecipe(id);
      remove(id);
    } catch (err) {
      alert('Failed to save: ' + err.message);
    } finally {
      setActing(a => ({ ...a, [id]: null }));
    }
  };

  const handleReject = async (id) => {
    setActing(a => ({ ...a, [id]: 'rejecting' }));
    try {
      await rejectPendingRecipe(id);
      remove(id);
    } catch (err) {
      alert('Failed to reject: ' + err.message);
    } finally {
      setActing(a => ({ ...a, [id]: null }));
    }
  };

  if (loading) return <p className="empty-state">Loading...</p>;

  return (
    <div className="pending-page">
      <h2>Pending Recipes</h2>
      {generatingMsg && (
        <div className="pending-generating-banner">
          <span className="pending-generating-spinner" />
          {generatingMsg}
        </div>
      )}
      <p className="pending-page-hint">
        These recipes were generated in the background. Save the ones you like to your library, or reject the rest.
        Pending recipes are automatically discarded after 7 days.
      </p>

      {recipes.length === 0 ? (
        <p className="empty-state">No pending recipes. Enable background generation in Settings to get some.</p>
      ) : (
        <div className="pending-list">
          {recipes.map(recipe => (
            <div key={recipe.id} className="pending-item">
              <RecipeCard recipe={recipe} showIngredients fetchImageEndpoint={`/pending/${recipe.id}/fetch-image`} />
              <p className="pending-item-date">
                Generated {new Date(recipe.created_at).toLocaleDateString(undefined, { dateStyle: 'medium' })}
              </p>
              <div className="pending-item-actions">
                <button
                  className="btn btn-primary"
                  onClick={() => handleApprove(recipe.id)}
                  disabled={!!acting[recipe.id]}
                >
                  {acting[recipe.id] === 'approving' ? 'Saving…' : 'Save to library'}
                </button>
                <button
                  className="btn btn-danger"
                  onClick={() => handleReject(recipe.id)}
                  disabled={!!acting[recipe.id]}
                >
                  {acting[recipe.id] === 'rejecting' ? 'Rejecting…' : 'Reject'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
