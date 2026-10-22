import { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { getRecipe, deleteRecipe } from '../api/client';
import { errorMessage, useFetch } from '../hooks/useFetch';
import RecipeDetail from '../components/RecipeDetail';

export default function RecipePage() {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const { data: recipe, loading, error: loadError } = useFetch(`recipe:${id}`, signal => getRecipe(id, signal));
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const handleDelete = async () => {
    if (!recipe || !confirm('Delete this recipe?')) return;
    try {
      await deleteRecipe(recipe.id);
      navigate('/library');
    } catch (err) {
      setDeleteError(errorMessage(err));
    }
  };

  if (loading) return <p>Loading...</p>;
  const error = loadError ?? deleteError;
  if (error) return <div className="error-message">{error}</div>;
  if (!recipe) return <p>Recipe not found.</p>;

  return (
    <div className="recipe-detail-page">
      {/* key resets local edit/image state when navigating between recipes */}
      <RecipeDetail key={recipe.id} recipe={recipe} />
      <div className="recipe-page-actions" style={{ padding: '0 24px', marginTop: 32 }}>
        <button className="btn btn-secondary" onClick={() => navigate('/library')}>← Back to library</button>
        <button className="btn btn-danger" onClick={handleDelete} style={{ marginLeft: 'auto' }}>Delete recipe</button>
      </div>
    </div>
  );
}
