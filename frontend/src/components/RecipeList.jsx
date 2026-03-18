import RecipeCard from './RecipeCard';

export default function RecipeList({ recipes, onDelete }) {
  if (recipes.length === 0) {
    return <p className="empty-state">No recipes found. Generate some!</p>;
  }

  return (
    <div className="recipe-list">
      {recipes.map(recipe => (
        <RecipeCard key={recipe.id} recipe={recipe} showLink onDelete={onDelete} />
      ))}
    </div>
  );
}
