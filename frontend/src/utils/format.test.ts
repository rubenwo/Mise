import { describe, expect, it } from 'vitest';
import { formatDate, planStatusLabel } from './format';

describe('format helpers', () => {
  it('labels plan statuses', () => {
    expect(planStatusLabel('draft')).toBe('Draft');
    expect(planStatusLabel('active')).toBe('Active');
    expect(planStatusLabel('completed')).toBe('Done');
  });

  it('returns null for missing dates', () => {
    expect(formatDate(undefined)).toBeNull();
    expect(formatDate('')).toBeNull();
    expect(formatDate('2026-03-04T12:00:00Z')).toMatch(/2026/);
  });
});
