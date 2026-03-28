import { useState, useCallback, useRef } from 'react';
import { generateStream } from '../api/client';

let _clientIdCounter = 0;

export function useGeneration() {
  const [events, setEvents] = useState([]);
  const [recipes, setRecipes] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  // Accumulates near_duplicate warnings keyed by event.index.
  // Using a ref so updates are visible synchronously when the recipe event
  // arrives in the same parsing loop iteration.
  const pendingWarnings = useRef({});

  const generate = useCallback(async (endpoint, body) => {
    setLoading(true);
    setError(null);
    setEvents([]);
    setRecipes([]);
    pendingWarnings.current = {};

    try {
      const response = await generateStream(endpoint, body);
      if (!response.ok) {
        const err = await response.json().catch(() => ({ error: response.statusText }));
        throw new Error(err.error || 'Generation failed');
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop();

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;
          try {
            const event = JSON.parse(line.slice(6));
            setEvents(prev => [...prev, event]);

            if (event.type === 'near_duplicate') {
              const idx = event.index ?? 0;
              pendingWarnings.current[idx] = [
                ...(pendingWarnings.current[idx] || []),
                event.data,
              ];
            }
            if (event.type === 'recipe') {
              const idx = event.index ?? 0;
              const warnings = pendingWarnings.current[idx] || [];
              delete pendingWarnings.current[idx];
              setRecipes(prev => [...prev, { ...event.data, _warnings: warnings, _clientId: ++_clientIdCounter }]);
            }
            if (event.type === 'error') {
              setError(event.message);
            }
          } catch {
            // skip malformed events
          }
        }
      }
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  const reset = useCallback(() => {
    setEvents([]);
    setRecipes([]);
    setError(null);
  }, []);

  const removeRecipe = useCallback((clientId) => {
    setRecipes(prev => prev.filter(r => r._clientId !== clientId));
  }, []);

  return { events, recipes, loading, error, generate, reset, removeRecipe };
}
