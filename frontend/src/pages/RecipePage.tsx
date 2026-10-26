import { useParams, useNavigate } from 'react-router-dom';
import { getRecipe, deleteRecipe } from '../api/client';
import { errorMessage, useFetch } from '../hooks/useFetch';
import RecipeDetail from '../components/RecipeDetail';
import { useIntent } from '../hooks/usePresence';

export default function RecipePage() {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  // The cooking chat is where waiting for the model to load hurts most, so a
  // recipe page warms it straight away.
  useIntent('chat');
  const { data: recipe, loading, error } = useFetch(`recipe:${id}`, signal => getRecipe(id, signal));

  const handleDelete = async () => {
    if (!recipe || !confirm(`Delete "${recipe.title}"? Its cook history and ratings are deleted too.`)) return;
    try {
      await deleteRecipe(recipe.id);
      navigate('/library');
    } catch (err) {
      alert('Failed to delete: ' + errorMessage(err));
    }
  };

  if (loading) return <p className="empty-state">Loading...</p>;
  if (error) return <div className="error-message">{error}</div>;
  if (!recipe) return <p className="empty-state">Recipe not found.</p>;

  // key resets local edit/image/servings state when navigating between recipes.
  return <RecipeDetail key={recipe.id} recipe={recipe} onDelete={handleDelete} />;
}
