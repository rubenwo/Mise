import { listInventory } from '../api/client';
import type { InventoryItem } from '../types';
import { createSharedResource } from './sharedResource';

const EMPTY: InventoryItem[] = [];

// One fetch per session so RecipeCard/RecipeDetail "in stock" badges don't
// each hit the API.
const inventory = createSharedResource<InventoryItem[]>(async () => (await listInventory()) ?? [], EMPTY);

export const useInventory = inventory.useResource;

/** Call after any inventory mutation so stock badges reflect reality. */
export const revalidateInventory = inventory.revalidate;
