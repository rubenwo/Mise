import { describe, expect, it } from 'vitest';
import { parseDays, toggleDay } from './useSettingsForm';

describe('weekday helpers', () => {
  it('parses comma-separated days, ignoring blanks', () => {
    expect([...parseDays(' 1, 2,,5 ')]).toEqual(['1', '2', '5']);
    expect(parseDays('').size).toBe(0);
  });

  it('toggles a day on and off', () => {
    expect(toggleDay('1,2', '3')).toBe('1,2,3');
    expect(toggleDay('1,2,3', '2')).toBe('1,3');
    expect(toggleDay('', '0')).toBe('0');
  });
});
