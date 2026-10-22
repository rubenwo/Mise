import { listRecipeEatCounts } from '../api/client';
import type { RecipeEatStats } from '../types';
import { createSharedResource } from './sharedResource';

export type EatCounts = Record<number, RecipeEatStats>;

const EMPTY: EatCounts = {};

// One fetch per session, shared by every RecipeCard, indexed by recipe_id
// for O(1) lookups.
const eatCounts = createSharedResource<EatCounts>(async () => {
  const stats = await listRecipeEatCounts();
  return Object.fromEntries((stats ?? []).map(s => [s.recipe_id, s]));
}, EMPTY);

export const useEatCounts = eatCounts.useResource;

/** Call after marking a recipe done, rating it, or ending a plan. */
export const revalidateEatCounts = eatCounts.revalidate;
