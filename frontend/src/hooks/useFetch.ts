import { useCallback, useEffect, useEffectEvent, useState, type SetStateAction } from 'react';

interface Settled<T> {
  key: string;
  version: number;
  data?: T;
  error?: string;
}

export interface FetchState<T> {
  /** Latest data for the current key. Kept while a reload is in flight. */
  data: T | undefined;
  /** Error message from the most recent attempt for the current key. */
  error: string | undefined;
  /** True until the request for the current key/version settles. */
  loading: boolean;
  /** Re-run the fetcher for the current key. */
  reload: () => void;
  /** Update the cached data locally, e.g. after a mutation response. */
  setData: (update: SetStateAction<T | undefined>) => void;
}

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/**
 * Loads data for `key` with `fetcher`, cancelling stale requests when the key
 * changes or the component unmounts. Pass `null` as the key to skip fetching.
 *
 * Loading state is derived from which key last settled, so the hook never
 * calls setState synchronously inside an effect.
 */
export function useFetch<T>(key: string | null, fetcher: (signal: AbortSignal) => Promise<T>): FetchState<T> {
  const [version, setVersion] = useState(0);
  const [settled, setSettled] = useState<Settled<T> | null>(null);
  const run = useEffectEvent(fetcher);

  useEffect(() => {
    if (key === null) return;
    const controller = new AbortController();
    run(controller.signal).then(
      data => setSettled({ key, version, data }),
      err => {
        if (!controller.signal.aborted) {
          setSettled(prev => ({ key, version, data: prev?.key === key ? prev.data : undefined, error: errorMessage(err) }));
        }
      },
    );
    return () => controller.abort();
  }, [key, version]);

  const sameKey = settled !== null && settled.key === key;
  const current = sameKey && settled.version === version;

  const reload = useCallback(() => setVersion(v => v + 1), []);
  const setData = useCallback((update: SetStateAction<T | undefined>) => {
    setSettled(prev => {
      if (!prev) return prev;
      const data = typeof update === 'function' ? (update as (p: T | undefined) => T | undefined)(prev.data) : update;
      return { ...prev, data };
    });
  }, []);

  return {
    data: sameKey ? settled.data : undefined,
    error: current ? settled.error : undefined,
    loading: key !== null && !current,
    reload,
    setData,
  };
}
