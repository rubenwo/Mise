export function fuzzyMatch(query, text) {
  if (!query || !text) return false;
  const q = query.toLowerCase();
  const t = text.toLowerCase();
  let qi = 0;
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) qi++;
  }
  return qi === q.length;
}

function recipeSearchText(recipe) {
  const parts = [
    recipe.title,
    recipe.description,
    recipe.cuisine_type,
    ...(recipe.tags || []),
    ...(recipe.ingredients || []).map(i => i.name),
  ];
  return parts.filter(Boolean).join(' ');
}

export function filterRecipes(query, recipes) {
  const trimmed = query.trim();
  if (!trimmed) return recipes;

  const words = trimmed.split(/\s+/);
  return recipes.filter(recipe => {
    const text = recipeSearchText(recipe);
    return words.every(word => fuzzyMatch(word, text));
  });
}
