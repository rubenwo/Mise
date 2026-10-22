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
