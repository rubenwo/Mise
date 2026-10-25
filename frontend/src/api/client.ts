import type {
  AggregatedIngredient,
  AHOrderResult,
  AppSetting,
  ChatMessage,
  CuisineMeta,
  FeatureStatus,
  InventoryInput,
  InventoryItem,
  LibrarySearchParams,
  MealPlan,
  PendingIngredientScan,
  PlanSuggestions,
  RecommendedRecipe,
  Provider,
  ProviderInput,
  ProviderType,
  Recipe,
  RecipeEatStats,
  RecipeHistoryEntry,
  RecipeList,
  RecommendResponse,
  UpdatePlanRecipeRequest,
} from '../types';

const API_BASE = '/api';

type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

interface RequestOptions {
  method?: Method;
  body?: unknown;
  signal?: AbortSignal;
}

async function errorFromResponse(res: Response, fallback: string): Promise<Error> {
  const body = await res.json().catch(() => ({ error: res.statusText }));
  return new Error(body.error || fallback);
}

async function request<T>(path: string, { method = 'GET', body, signal }: RequestOptions = {}): Promise<T> {
  const init: RequestInit = { method, signal };
  if (body instanceof FormData) {
    init.body = body;
  } else if (body !== undefined) {
    init.body = JSON.stringify(body);
    init.headers = { 'Content-Type': 'application/json' };
  }

  const res = await fetch(`${API_BASE}${path}`, init);
  if (!res.ok) throw await errorFromResponse(res, 'Request failed');
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

// ── Recipes ───────────────────────────────────────────
export function listRecipes(limit = 20, offset = 0, cuisineType = '', signal?: AbortSignal) {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  if (cuisineType) params.set('cuisine_type', cuisineType);
  return request<RecipeList>(`/recipes?${params}`, { signal });
}

export function listCuisines(signal?: AbortSignal) {
  return request<CuisineMeta[]>('/recipes/cuisines', { signal });
}

export function librarySearch(params: LibrarySearchParams, signal?: AbortSignal) {
  return request<RecipeList>('/recipes/library-search', { method: 'POST', body: params, signal });
}

export function getRecipe(id: number | string, signal?: AbortSignal) {
  return request<Recipe>(`/recipes/${id}`, { signal });
}

export function saveRecipe(recipe: Partial<Recipe>) {
  return request<Recipe>('/recipes', { method: 'POST', body: recipe });
}

export function deleteRecipe(id: number) {
  return request<void>(`/recipes/${id}`, { method: 'DELETE' });
}

export function updateRecipeContent(id: number, ingredients: Recipe['ingredients'], instructions: string[], cuisineType: string) {
  return request<void>(`/recipes/${id}`, {
    method: 'PATCH',
    body: { ingredients, instructions, cuisine_type: cuisineType },
  });
}

export function recommendRecipes(messages: ChatMessage[], limit?: number) {
  return request<RecommendResponse>('/recipes/recommend', { method: 'POST', body: { messages, limit } });
}

export function fetchRecipeImage(id: number) {
  return request<{ image_url: string }>(`/recipes/${id}/fetch-image`, { method: 'POST' });
}

export function previewImageByTitle(title: string, signal?: AbortSignal) {
  return request<{ image_url: string }>('/recipes/preview-image', { method: 'POST', body: { title }, signal });
}

export function getChatHistory(recipeId: number, signal?: AbortSignal) {
  return request<ChatMessage[]>(`/recipes/${recipeId}/chat`, { signal });
}

export function sendChatMessage(recipeId: number, message: string) {
  return request<{ response: string }>(`/recipes/${recipeId}/chat`, { method: 'POST', body: { message } });
}

export function findDuplicates(signal?: AbortSignal) {
  return request<{ groups: Recipe[][] }>('/recipes/duplicates', { signal });
}

/** Every plan a recipe has appeared on, with completion and rating. */
export function getRecipeHistory(id: number, signal?: AbortSignal) {
  return request<RecipeHistoryEntry[]>(`/recipes/${id}/history`, { signal });
}

/** Library-wide count, last-cooked date and average rating per recipe. */
export function listRecipeEatCounts(signal?: AbortSignal) {
  return request<RecipeEatStats[]>('/recipes/eat-counts', { signal });
}

// ── Meal plans ────────────────────────────────────────
export function createPlan(name: string) {
  return request<MealPlan>('/plans', { method: 'POST', body: { name } });
}

export function listPlans(signal?: AbortSignal) {
  return request<MealPlan[]>('/plans', { signal });
}

export function getPlan(id: number | string, signal?: AbortSignal) {
  return request<MealPlan>(`/plans/${id}`, { signal });
}

export function updatePlan(id: number, data: { status?: MealPlan['status'] }) {
  return request<MealPlan>(`/plans/${id}`, { method: 'PATCH', body: data });
}

export function deletePlan(id: number) {
  return request<void>(`/plans/${id}`, { method: 'DELETE' });
}

export function addRecipeToPlan(planId: number, recipeId: number, servings: number) {
  return request<MealPlan>(`/plans/${planId}/recipes`, {
    method: 'POST',
    body: { recipe_id: recipeId, servings },
  });
}

export function removeRecipeFromPlan(planId: number, recipeId: number) {
  return request<MealPlan>(`/plans/${planId}/recipes/${recipeId}`, { method: 'DELETE' });
}

export function updatePlanRecipe(planId: number, recipeId: number, data: UpdatePlanRecipeRequest) {
  return request<MealPlan>(`/plans/${planId}/recipes/${recipeId}`, { method: 'PATCH', body: data });
}

export function getPlanSuggestions(planId: number, ingredients: string[]) {
  return request<PlanSuggestions>(`/plans/${planId}/suggestions`, { method: 'POST', body: { ingredients } });
}

export function getPlanIngredients(planId: number | string, signal?: AbortSignal) {
  return request<AggregatedIngredient[]>(`/plans/${planId}/ingredients`, { signal });
}

export function randomizePlan(planId: number, servings: number[], excludedIds: number[] = []) {
  return request<MealPlan>(`/plans/${planId}/randomize`, {
    method: 'POST',
    body: { servings, excluded_ids: excludedIds },
  });
}

export function orderPlanAH(planId: number) {
  return request<AHOrderResult>(`/plans/${planId}/order/ah`, { method: 'POST' });
}

/** The plan's last Albert Heijn search, or undefined if it was never searched. */
export function getLastAHSearch(planId: number, signal?: AbortSignal) {
  return request<AHOrderResult | undefined>(`/plans/${planId}/order/ah`, { signal });
}

export function getGroceryChecks(planId: number, signal?: AbortSignal) {
  return request<string[]>(`/plans/${planId}/grocery-checks`, { signal });
}

export function setGroceryCheck(planId: number, name: string, checked: boolean) {
  return request<void>(`/plans/${planId}/grocery-checks`, { method: 'PUT', body: { name, checked } });
}

// ── Recommendations ───────────────────────────────────
export interface RecommendationFilters {
  cuisine?: string;
  max_minutes?: number;
  dietary?: string;
}

export function getRecommendations(
  { limit = 12, offset = 0, ...filters }: RecommendationFilters & { limit?: number; offset?: number },
  signal?: AbortSignal,
) {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  for (const [k, v] of Object.entries(filters)) if (v) params.set(k, String(v));
  return request<RecommendedRecipe[]>(`/recommendations?${params}`, { signal });
}

export function recordImpressions(recipeIds: number[]) {
  return request<void>('/recommendations/impressions', { method: 'POST', body: { recipe_ids: recipeIds } });
}

export function getFavourites(signal?: AbortSignal) {
  return request<Recipe[]>('/recipes/favourites', { signal });
}

// ── Settings ──────────────────────────────────────────
export function listProviders(signal?: AbortSignal) {
  return request<Provider[]>('/settings/providers', { signal });
}

export function listModels(host: string, type: ProviderType = 'ollama', signal?: AbortSignal) {
  const params = new URLSearchParams({ host, type });
  return request<string[]>(`/settings/models?${params}`, { signal });
}

export function createProvider(provider: ProviderInput) {
  return request<Provider>('/settings/providers', { method: 'POST', body: provider });
}

export function updateProvider(id: number, provider: ProviderInput) {
  return request<Provider>(`/settings/providers/${id}`, { method: 'PATCH', body: provider });
}

export function deleteProvider(id: number) {
  return request<void>(`/settings/providers/${id}`, { method: 'DELETE' });
}

export function getSettings(signal?: AbortSignal) {
  return request<AppSetting[]>('/settings', { signal });
}

export function getFeatureStatus(signal?: AbortSignal) {
  return request<Record<string, FeatureStatus>>('/settings/features', { signal });
}

export function updateSettings(settings: Record<string, string>) {
  return request<AppSetting[]>('/settings', { method: 'PATCH', body: settings });
}

export function runTranslationNow() {
  return request<{ translated: number; message?: string }>('/settings/translation/run', { method: 'POST' });
}

// ── Pending recipes (background-generated, awaiting review) ──
export function listPendingRecipes(signal?: AbortSignal) {
  return request<Recipe[]>('/pending', { signal });
}

export function approvePendingRecipe(id: number) {
  return request<Recipe>(`/pending/${id}/approve`, { method: 'POST' });
}

export function rejectPendingRecipe(id: number) {
  return request<void>(`/pending/${id}`, { method: 'DELETE' });
}

export function updatePendingRecipeContent(id: number, ingredients: Recipe['ingredients'], instructions: string[]) {
  return request<void>(`/pending/${id}`, { method: 'PATCH', body: { ingredients, instructions } });
}

export function fetchPendingImage(id: number) {
  return request<{ image_url: string }>(`/pending/${id}/fetch-image`, { method: 'POST' });
}

// ── Generation (SSE) ──────────────────────────────────
export type GenerateEndpoint = 'single' | 'batch' | 'refine' | 'import';

export async function generateStream(endpoint: GenerateEndpoint, body: unknown, signal?: AbortSignal) {
  const res = await fetch(`${API_BASE}/generate/${endpoint}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok) throw await errorFromResponse(res, 'Generation failed');
  return res;
}

// ── Inventory ─────────────────────────────────────────
export function listInventory(signal?: AbortSignal) {
  return request<InventoryItem[]>('/inventory', { signal });
}

export function createInventoryItem(item: InventoryInput) {
  return request<InventoryItem>('/inventory', { method: 'POST', body: item });
}

export function updateInventoryItem(id: number, item: Partial<InventoryInput>) {
  return request<InventoryItem>(`/inventory/${id}`, { method: 'PATCH', body: item });
}

export function deleteInventoryItem(id: number) {
  return request<void>(`/inventory/${id}`, { method: 'DELETE' });
}

export function scanIngredient(imageFile: File) {
  const form = new FormData();
  form.append('image', imageFile);
  return request<PendingIngredientScan[]>('/inventory/scan', { method: 'POST', body: form });
}

export function listPendingScans(signal?: AbortSignal) {
  return request<PendingIngredientScan[]>('/inventory/scans', { signal });
}

export function deletePendingScan(id: number) {
  return request<void>(`/inventory/scans/${id}`, { method: 'DELETE' });
}
