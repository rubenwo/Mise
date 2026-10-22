import { useCallback, useEffect, useRef, useState } from 'react';
import { generateStream, type GenerateEndpoint } from '../api/client';
import { errorMessage } from './useFetch';
import type { GeneratedRecipe, NearDuplicate, Recipe, SSEEvent } from '../types';

let clientIdCounter = 0;

/**
 * Splits buffered SSE text into complete `data:` events. Returns the parsed
 * events plus any trailing partial line to prepend to the next chunk.
 * Malformed event payloads are skipped.
 */
export function parseSSEChunk(buffer: string): { events: SSEEvent[]; rest: string } {
  const lines = buffer.split('\n');
  const rest = lines.pop() ?? '';
  const events: SSEEvent[] = [];
  for (const line of lines) {
    if (!line.startsWith('data: ')) continue;
    try {
      events.push(JSON.parse(line.slice(6)));
    } catch {
      // skip malformed events
    }
  }
  return { events, rest };
}

export function useGeneration() {
  const [events, setEvents] = useState<SSEEvent[]>([]);
  const [recipes, setRecipes] = useState<GeneratedRecipe[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // near_duplicate warnings keyed by batch slot. A ref so they are visible
  // synchronously when the recipe event arrives in the same parsing loop.
  const pendingWarnings = useRef<Record<number, NearDuplicate[]>>({});
  // AbortController for the in-flight stream, so unmounting or starting a
  // new generation cancels the previous one.
  const abortRef = useRef<AbortController | null>(null);

  const handleEvent = useCallback((event: SSEEvent) => {
    setEvents(prev => [...prev, event]);
    const idx = event.index ?? 0;

    switch (event.type) {
      case 'near_duplicate':
        pendingWarnings.current[idx] = [...(pendingWarnings.current[idx] ?? []), event.data as NearDuplicate];
        break;
      case 'recipe': {
        const warnings = pendingWarnings.current[idx] ?? [];
        delete pendingWarnings.current[idx];
        const recipe = event.data as Recipe;
        setRecipes(prev => [...prev, { ...recipe, _warnings: warnings, _clientId: ++clientIdCounter }]);
        break;
      }
      case 'error':
        setError(event.message ?? 'Generation failed');
        break;
    }
  }, []);

  const generate = useCallback(async (endpoint: GenerateEndpoint, body: unknown) => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    setLoading(true);
    setError(null);
    setEvents([]);
    setRecipes([]);
    pendingWarnings.current = {};

    try {
      const response = await generateStream(endpoint, body, controller.signal);
      const reader = response.body!.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        const parsed = parseSSEChunk(buffer + decoder.decode(value, { stream: true }));
        buffer = parsed.rest;
        parsed.events.forEach(handleEvent);
      }
    } catch (err) {
      // AbortError is expected when the user navigates away or restarts.
      if (!(err instanceof DOMException && err.name === 'AbortError')) setError(errorMessage(err));
    } finally {
      if (abortRef.current === controller) abortRef.current = null;
      setLoading(false);
    }
  }, [handleEvent]);

  useEffect(() => () => abortRef.current?.abort(), []);

  const reset = useCallback(() => {
    setEvents([]);
    setRecipes([]);
    setError(null);
  }, []);

  const removeRecipe = useCallback((clientId: number) => {
    setRecipes(prev => prev.filter(r => r._clientId !== clientId));
  }, []);

  return { events, recipes, loading, error, generate, reset, removeRecipe };
}
