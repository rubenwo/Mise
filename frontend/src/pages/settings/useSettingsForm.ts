import { useState } from 'react';
import { getSettings, updateSettings } from '../../api/client';
import { errorMessage, useFetch } from '../../hooks/useFetch';

/**
 * Edits a subset of app settings. Values come from (in order of precedence)
 * local edits, the stored settings, then `defaults`.
 *
 * With `saveDefaults: false`, keys that were never stored or edited are left
 * out of the save so backend/config defaults keep applying.
 */
export function useSettingsForm<K extends string>(defaults: Record<K, string>, { saveDefaults = true } = {}) {
  const query = useFetch('settings', getSettings);
  const [edits, setEdits] = useState<Partial<Record<K, string>>>({});
  const [saving, setSaving] = useState(false);

  const keys = Object.keys(defaults) as K[];
  const stored: Partial<Record<K, string>> = {};
  for (const s of query.data ?? []) {
    if (s.key in defaults) stored[s.key as K] = s.value;
  }
  const values = { ...defaults, ...stored, ...edits };

  const set = (key: K, value: string) => setEdits(prev => ({ ...prev, [key]: value }));

  const save = async () => {
    setSaving(true);
    try {
      const toSave = saveDefaults ? keys : keys.filter(k => k in stored || k in edits);
      await updateSettings(Object.fromEntries(toSave.map(k => [k, values[k]])));
    } catch (err) {
      alert('Failed to save settings: ' + errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return { values, set, save, saving, loading: query.loading && !query.data };
}

// Weekday order for display: Mon–Sun. Values match Go's time.Weekday (0=Sunday).
export const WEEKDAYS = [
  { label: 'Mon', value: '1' },
  { label: 'Tue', value: '2' },
  { label: 'Wed', value: '3' },
  { label: 'Thu', value: '4' },
  { label: 'Fri', value: '5' },
  { label: 'Sat', value: '6' },
  { label: 'Sun', value: '0' },
];

export function parseDays(val: string | undefined): Set<string> {
  if (!val) return new Set();
  return new Set(val.split(',').map(d => d.trim()).filter(Boolean));
}

/** Returns the comma-separated day list with `day` toggled. */
export function toggleDay(days: string, day: string): string {
  const next = parseDays(days);
  if (next.has(day)) next.delete(day);
  else next.add(day);
  return [...next].join(',');
}
