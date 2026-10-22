// @vitest-environment jsdom
import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useFetch } from './useFetch';

describe('useFetch', () => {
  it('loads data for a key', async () => {
    const { result } = renderHook(() => useFetch('a', async () => 42));
    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toBe(42);
    expect(result.current.error).toBeUndefined();
  });

  it('skips fetching when the key is null', () => {
    const fetcher = vi.fn(async () => 1);
    const { result } = renderHook(() => useFetch(null, fetcher));
    expect(result.current.loading).toBe(false);
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('reports errors as messages', async () => {
    const { result } = renderHook(() => useFetch('err', async () => { throw new Error('boom'); }));
    await waitFor(() => expect(result.current.error).toBe('boom'));
    expect(result.current.loading).toBe(false);
  });

  it('aborts the previous request and drops stale data when the key changes', async () => {
    const signals: AbortSignal[] = [];
    const { result, rerender } = renderHook(({ k }) => useFetch(k, async signal => {
      signals.push(signal);
      return k;
    }), { initialProps: { k: 'one' } });
    await waitFor(() => expect(result.current.data).toBe('one'));

    rerender({ k: 'two' });
    expect(result.current.data).toBeUndefined();
    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.data).toBe('two'));
    expect(signals[0].aborted).toBe(true);
  });

  it('keeps data visible while reloading and supports local updates', async () => {
    let n = 0;
    const { result } = renderHook(() => useFetch('counter', async () => ++n));
    await waitFor(() => expect(result.current.data).toBe(1));

    act(() => result.current.reload());
    expect(result.current.loading).toBe(true);
    expect(result.current.data).toBe(1);
    await waitFor(() => expect(result.current.data).toBe(2));

    act(() => result.current.setData(v => (v ?? 0) + 10));
    expect(result.current.data).toBe(12);
  });
});
