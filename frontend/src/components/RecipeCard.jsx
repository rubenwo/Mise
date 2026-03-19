import { useState } from 'react';
import { Link } from 'react-router-dom';
import { fetchRecipeImage } from '../api/client';

export default function RecipeCard({ recipe: initialRecipe, showLink = false, showIngredients = false, onDelete, fetchImageEndpoint }) {
  const [recipe, setRecipe] = useState(initialRecipe);
  const [fetchingImage, setFetchingImage] = useState(false);

  const handleFetchImage = async (e) => {
    e.preventDefault();
    setRecipe(r => ({ ...r, image_url: '' }));
    setFetchingImage(true);
    try {
      // Allow callers to override the endpoint (e.g. pending recipes use /api/pending/{id}/fetch-image).
      const result = fetchImageEndpoint
        ? await fetch(`/api${fetchImageEndpoint}`, { method: 'POST' }).then(r => r.json())
        : await fetchRecipeImage(recipe.id);
      setRecipe(r => ({ ...r, image_url: result.image_url }));
    } catch (err) {
      console.error('Image fetch failed:', err);
    } finally {
      setFetchingImage(false);
    }
  };

  return (
    <div className="recipe-card">
      {recipe.image_url && (
        <img className="recipe-card-image" src={recipe.image_url} alt={recipe.title} loading="lazy" />
      )}
      <div className="recipe-card-header">
        <h3>{recipe.title}</h3>
        {recipe.cuisine_type && <span className="cuisine-badge">{recipe.cuisine_type}</span>}
      </div>
      <p className="recipe-description">{recipe.description}</p>
      <div className="recipe-meta">
        {recipe.prep_time_minutes > 0 && <span>{'\u23F1'} Prep: {recipe.prep_time_minutes}m</span>}
        {recipe.cook_time_minutes > 0 && <span>{'\uD83D\uDD25'} Cook: {recipe.cook_time_minutes}m</span>}
        <span>{'\uD83C\uDF7D'} Servings: {recipe.servings}</span>
        {recipe.difficulty && <span className={`difficulty difficulty-${recipe.difficulty}`}>{recipe.difficulty}</span>}
      </div>
      {showIngredients && recipe.ingredients && recipe.ingredients.length > 0 && (
        <div className="recipe-card-ingredients">
          <h4>Ingredients</h4>
          <ul className="ingredients-list">
            {recipe.ingredients.map((ing, i) => (
              <li key={i}>
                <strong>{ing.amount} {ing.unit}</strong> {ing.name}
                {ing.notes && <span className="ingredient-notes"> ({ing.notes})</span>}
              </li>
            ))}
          </ul>
        </div>
      )}
      {recipe.tags && recipe.tags.length > 0 && (
        <div className="recipe-tags">
          {recipe.tags.map(tag => <span key={tag} className="tag">{tag}</span>)}
        </div>
      )}
      <div className="recipe-card-actions">
        {showLink && recipe.id && <Link to={`/recipe/${recipe.id}`} className="btn btn-secondary">View</Link>}
        {recipe.id && (
          <button className="btn btn-secondary" onClick={handleFetchImage} disabled={fetchingImage}>
            {fetchingImage ? 'Fetching...' : 'Refresh Image'}
          </button>
        )}
        {onDelete && <button className="btn btn-danger" onClick={() => onDelete(recipe.id)}>Delete</button>}
      </div>
    </div>
  );
}
