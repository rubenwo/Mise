import { useEffect, useSyncExternalStore } from 'react';

/**
 * A module-level cache shared by every component that reads it, loaded at
 * most once per session unless explicitly revalidated.
 */
export function createSharedResource<T>(load: () => Promise<T>, fallback: T) {
  let value: T | null = null;
  let inFlight = false;
  const listeners = new Set<() => void>();

  const notify = () => listeners.forEach(fn => fn());

  function fetchValue(force = false) {
    if (inFlight || (!force && value !== null)) return;
    inFlight = true;
    load()
      .then(v => { value = v; })
      .catch(() => { value = value ?? fallback; })
      .finally(() => {
        inFlight = false;
        notify();
      });
  }

  function subscribe(listener: () => void) {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
  }

  const getSnapshot = () => value ?? fallback;

  function useResource(): T {
    const snapshot = useSyncExternalStore(subscribe, getSnapshot);
    useEffect(() => fetchValue(), []);
    return snapshot;
  }

  /** Refetch after a mutation so every reader sees fresh data. */
  const revalidate = () => fetchValue(true);

  return { useResource, revalidate };
}
