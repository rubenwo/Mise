// @vitest-environment jsdom
import { renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { startPresence, useIntent } from './usePresence';

function bodies(fetchMock: ReturnType<typeof vi.fn>) {
  return fetchMock.mock.calls.map(([, init]) => JSON.parse((init as RequestInit).body as string));
}

describe('presence', () => {
  let fetchMock: ReturnType<typeof vi.fn>;
  let stop: () => void;

  beforeEach(() => {
    fetchMock = vi.fn(() => Promise.resolve(new Response(null, { status: 204 })));
    vi.stubGlobal('fetch', fetchMock);
    stop = startPresence();
  });
  afterEach(() => {
    stop();
    vi.unstubAllGlobals();
  });

  it('reports a mounted intent and drops it on unmount', () => {
    const { unmount } = renderHook(() => useIntent('chat'));
    expect(bodies(fetchMock).at(-1)).toEqual({ active: true, intents: ['chat'] });
    unmount();
    renderHook(() => useIntent('search', false));
    renderHook(() => useIntent('generation'));
    expect(bodies(fetchMock).at(-1)).toEqual({ active: true, intents: ['generation'] });
  });

  it('reports leaving with a beacon when the tab is hidden', () => {
    const beacon = vi.fn(() => true);
    vi.stubGlobal('navigator', { ...navigator, sendBeacon: beacon });
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
    document.dispatchEvent(new Event('visibilitychange'));
    expect(beacon).toHaveBeenCalledWith('/api/presence', expect.any(Blob));
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
  });
});
