function normalize(name) {
  return name.toLowerCase().trim().replace(/\s+/g, ' ');
}

// Returns true if the ingredient name matches any inventory item.
// Bidirectional substring: "chicken breast" matches inventory "chicken",
// and inventory "whole milk" matches recipe ingredient "milk".
export function isInStock(ingredientName, inventoryItems) {
  if (!inventoryItems || inventoryItems.length === 0) return false;
  const ingNorm = normalize(ingredientName);
  return inventoryItems.some(item => {
    const itemNorm = normalize(item.name);
    return ingNorm.includes(itemNorm) || itemNorm.includes(ingNorm);
  });
}

// Returns ingredients annotated with inStock, or null if inventory is empty.
export function matchIngredients(ingredients, inventoryItems) {
  if (!inventoryItems || inventoryItems.length === 0 || !ingredients || !ingredients.length) return null;
  return ingredients.map(ing => ({ ...ing, inStock: isInStock(ing.name, inventoryItems) }));
}

// Returns { total, inStock, missing } or null if no matched data.
export function stockSummary(matched) {
  if (!matched) return null;
  const inStock = matched.filter(i => i.inStock).length;
  return { total: matched.length, inStock, missing: matched.length - inStock };
}
