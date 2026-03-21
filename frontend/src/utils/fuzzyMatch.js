function recipeSearchText(recipe) {
  const parts = [
    recipe.title,
    recipe.description,
    recipe.cuisine_type,
    ...(recipe.tags || []),
    ...(recipe.ingredients || []).map(i => i.name),
  ];
  return parts.filter(Boolean).join(' ').toLowerCase();
}

export function filterRecipes(query, recipes) {
  const trimmed = query.trim();
  if (!trimmed) return recipes;

  const words = trimmed.toLowerCase().split(/\s+/);
  return recipes.filter(recipe => {
    const text = recipeSearchText(recipe);
    return words.every(word => text.includes(word));
  });
}
