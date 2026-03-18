const API_BASE = '/api';

async function request(path, options = {}) {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || 'Request failed');
  }
  if (res.status === 204) return null;
  return res.json();
}

export function listRecipes(limit = 20, offset = 0) {
  return request(`/recipes?limit=${limit}&offset=${offset}`);
}

export function getRecipe(id) {
  return request(`/recipes/${id}`);
}

export function saveRecipe(recipe) {
  return request('/recipes', {
    method: 'POST',
    body: JSON.stringify(recipe),
  });
}

export function deleteRecipe(id) {
  return request(`/recipes/${id}`, { method: 'DELETE' });
}

export function searchRecipes(params) {
  return request('/recipes/search', {
    method: 'POST',
    body: JSON.stringify(params),
  });
}

export function generateStream(endpoint, body) {
  return fetch(`${API_BASE}/generate/${endpoint}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}
