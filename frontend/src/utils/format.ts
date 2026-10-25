import type { PlanStatus } from '../types';

/** "Mar 4, 2026" in the user's locale, or null when there is no date. */
export function formatDate(iso: string | undefined | null): string | null {
  if (!iso) return null;
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

const PLAN_STATUS_LABELS: Record<PlanStatus, string> = { draft: 'Draft', active: 'Active', completed: 'Done' };

export function planStatusLabel(status: PlanStatus): string {
  return PLAN_STATUS_LABELS[status] ?? status;
}

const dmy = (d: Date) =>
  [d.getDate(), d.getMonth() + 1].map(n => String(n).padStart(2, '0')).join('-') + `-${d.getFullYear()}`;

/** Default plan name in the user's convention: the two-week grocery period, "DD-MM-YYYY tot DD-MM-YYYY". */
export function defaultPlanName(start = new Date()): string {
  const end = new Date(start);
  end.setDate(end.getDate() + 14);
  return `${dmy(start)} tot ${dmy(end)}`;
}

export const servingsLabel = (n: number) => `${n} ${n === 1 ? 'serving' : 'servings'}`;

/**
 * Scales an ingredient amount and rounds it the way a cook would write it:
 * whole items and spoons to the nearest quarter, small weights to whole
 * numbers, larger ones to the nearest 5.
 */
export function scaleAmount(amount: number, factor: number, unit: string): string {
  if (!amount) return '';
  const v = amount * factor;
  const u = unit.toLowerCase();
  let rounded: number;
  if (u === '' || u === 'tsp' || u === 'tbsp' || u === 'clove' || u === 'stalk') {
    rounded = Math.max(0.25, Math.round(v * 4) / 4);
  } else if (v >= 100) {
    rounded = Math.round(v / 5) * 5;
  } else if (v >= 10) {
    rounded = Math.round(v);
  } else {
    rounded = Math.round(v * 10) / 10;
  }
  return String(rounded);
}
