// API types mirroring the Go models in backend/internal/models.
// Timestamps arrive as RFC 3339 strings.

export interface Ingredient {
  name: string;
  amount: number;
  unit: string;
  notes?: string;
}

export interface Recipe {
  id: number;
  title: string;
  description: string;
  cuisine_type: string;
  prep_time_minutes: number;
  cook_time_minutes: number;
  servings: number;
  difficulty: string;
  ingredients: Ingredient[];
  instructions: string[];
  dietary_restrictions: string[];
  tags: string[];
  generated_by_model: string;
  generation_prompt: string;
  image_url?: string;
  created_at: string;
  updated_at: string;
}

export interface NearDuplicate {
  id: number;
  title: string;
}

/** A generated recipe that has not been saved yet, plus client-side bookkeeping. */
export interface GeneratedRecipe extends Recipe {
  _clientId: number;
  _warnings: NearDuplicate[];
}

export interface RecipeList {
  recipes: Recipe[];
  total: number;
}

export interface CuisineMeta {
  cuisine_type: string;
  count: number;
  preview_images: string[];
}

export interface GenerateRequest {
  cuisine_type: string;
  dietary_restrictions: string[];
  max_prep_time: number;
  difficulty: string;
  servings: number;
  additional_notes: string;
}

export interface BatchGenerateRequest extends GenerateRequest {
  count: number;
}

export interface LibrarySearchParams {
  keywords?: string;
  cuisine_type?: string;
  dietary_restrictions?: string[];
  tags?: string[];
  max_total_minutes?: number;
  limit?: number;
}

export interface ChatMessage {
  id?: number;
  recipe_id?: number;
  role: 'user' | 'assistant';
  content: string;
  created_at?: string;
}

export interface RecommendResponse {
  message: string;
  recipes: Recipe[];
}

export type PlanStatus = 'draft' | 'active' | 'completed';

export interface MealPlanRecipe {
  id: number;
  recipe_id: number;
  servings: number;
  sort_order: number;
  completed: boolean;
  skipped: boolean;
  completed_at?: string;
  rating?: number;
  recipe: Recipe;
}

export interface MealPlan {
  id: number;
  name: string;
  status: PlanStatus;
  recipes?: MealPlanRecipe[];
  created_at: string;
  updated_at: string;
}

export interface UpdatePlanRecipeRequest {
  servings?: number;
  completed?: boolean;
  skipped?: boolean;
  rating?: number;
}

export interface AggregatedIngredient {
  name: string;
  amount: number;
  unit: string;
  recipes: string[];
}

export interface AHProduct {
  id: string;
  title: string;
  price: number;
  unit_size: string;
  image_url: string;
  url: string;
}

export interface AHOrderResult {
  matched: { ingredient: AggregatedIngredient; product: AHProduct }[];
  not_found: AggregatedIngredient[];
}

export interface PlanSuggestions {
  db_recipes: Recipe[];
  generated_recipe?: Recipe;
}

export interface RecipeHistoryEntry {
  plan_id: number;
  plan_name: string;
  plan_status: PlanStatus;
  completed: boolean;
  completed_at?: string;
  rating?: number;
}

export interface RecipeEatStats {
  recipe_id: number;
  count: number;
  last_cooked_at?: string;
  avg_rating?: number;
  rated_count: number;
}

export type ProviderType = 'ollama' | 'openai_compat';

export interface Provider {
  id: number;
  name: string;
  host: string;
  model: string;
  provider_type: ProviderType;
  enabled: boolean;
  created_at: string;
  health_status: string;
  last_health_check: string | null;
  last_error: string | null;
  tags: string[];
}

export type ProviderInput = Pick<Provider, 'name' | 'host' | 'model' | 'provider_type' | 'enabled' | 'tags'>;

export interface AppSetting {
  key: string;
  value: string;
}

export type FeatureStatus = 'available' | 'offline' | 'unconfigured';

export interface InventoryItem {
  id: number;
  name: string;
  amount: number;
  unit: string;
  notes: string;
  updated_at: string;
}

export type InventoryInput = Pick<InventoryItem, 'name' | 'amount' | 'unit' | 'notes'>;

export interface PendingIngredientScan {
  id: number;
  name: string;
  amount: number;
  unit: string;
  confident: boolean;
  created_at: string;
}

export type SSEEventType = 'status' | 'tool_call' | 'tool_result' | 'recipe' | 'near_duplicate' | 'error' | 'pending_added';

export interface SSEEvent {
  type: SSEEventType | (string & {});
  message?: string;
  tool?: string;
  args?: unknown;
  data?: unknown;
  index?: number;
}
