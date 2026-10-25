import { describe, expect, it } from 'vitest';
import { defaultPlanName, formatDate, planStatusLabel, scaleAmount } from './format';

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

  it('names plans after the two-week grocery period', () => {
    expect(defaultPlanName(new Date(2026, 9, 25))).toBe('25-10-2026 tot 08-11-2026');
    expect(defaultPlanName(new Date(2026, 11, 28))).toBe('28-12-2026 tot 11-01-2027');
  });

  it('scales amounts with cook-friendly rounding', () => {
    expect(scaleAmount(0.25, 2, '')).toBe('0.5');
    expect(scaleAmount(1, 1.5, 'clove')).toBe('1.5');
    expect(scaleAmount(150, 3, 'g')).toBe('450');
    expect(scaleAmount(75, 1.33, 'g')).toBe('100');
    expect(scaleAmount(15, 0.5, 'ml')).toBe('7.5');
    expect(scaleAmount(0, 2, 'g')).toBe('');
  });
});
