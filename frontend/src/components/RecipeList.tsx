import { Link } from 'react-router-dom';
import type { Recipe } from '../types';
import RecipeCard from './RecipeCard';

interface RecipeListProps {
  recipes: Recipe[];
  onDelete?: (id: number) => void;
  searchQuery?: string;
}

export default function RecipeList({ recipes, onDelete, searchQuery = '' }: RecipeListProps) {
  if (recipes.length === 0) {
    const params = new URLSearchParams({ mode: 'generate' });
    if (searchQuery.trim()) params.set('notes', searchQuery.trim());
    return (
      <div className="empty-state">
        <p>No recipes found.</p>
        <Link to={`/recipe/new?${params}`} className="btn btn-primary">Generate some!</Link>
      </div>
    );
  }

  return (
    <div className="recipe-list">
      {recipes.map(recipe => (
        <RecipeCard key={recipe.id} recipe={recipe} showLink onDelete={onDelete} />
      ))}
    </div>
  );
}
