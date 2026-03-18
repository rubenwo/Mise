import { Link } from 'react-router-dom';

export default function RecipeCard({ recipe, showLink = false, showIngredients = false, onDelete }) {
  return (
    <div className="recipe-card">
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
        {onDelete && <button className="btn btn-danger" onClick={() => onDelete(recipe.id)}>Delete</button>}
      </div>
    </div>
  );
}
