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
