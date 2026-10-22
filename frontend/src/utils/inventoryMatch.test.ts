import { describe, expect, it } from 'vitest';
import { isInStock, matchIngredients, stockSummary } from './inventoryMatch';

const inventory = [{ name: 'Chicken' }, { name: 'whole  milk' }];

describe('isInStock', () => {
  it('matches when the ingredient contains an inventory item', () => {
    expect(isInStock('chicken breast', inventory)).toBe(true);
  });

  it('matches when an inventory item contains the ingredient', () => {
    expect(isInStock('Milk', inventory)).toBe(true);
  });

  it('returns false for unrelated ingredients or empty inventory', () => {
    expect(isInStock('rice', inventory)).toBe(false);
    expect(isInStock('rice', [])).toBe(false);
  });
});

describe('matchIngredients / stockSummary', () => {
  it('annotates ingredients and summarises stock', () => {
    const matched = matchIngredients(
      [
        { name: 'chicken thigh', amount: 500, unit: 'g' },
        { name: 'rice', amount: 200, unit: 'g' },
      ],
      inventory,
    );
    expect(matched?.map(i => i.inStock)).toEqual([true, false]);
    expect(stockSummary(matched)).toEqual({ total: 2, inStock: 1, missing: 1 });
  });

  it('returns null without inventory or ingredients', () => {
    expect(matchIngredients([{ name: 'rice', amount: 1, unit: '' }], [])).toBeNull();
    expect(matchIngredients([], inventory)).toBeNull();
    expect(stockSummary(null)).toBeNull();
  });
});
