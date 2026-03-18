import { useState, useCallback } from 'react';
import { generateStream } from '../api/client';

export function useGeneration() {
  const [events, setEvents] = useState([]);
  const [recipes, setRecipes] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const generate = useCallback(async (endpoint, body) => {
    setLoading(true);
    setError(null);
    setEvents([]);
    setRecipes([]);

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

            if (event.type === 'recipe') {
              setRecipes(prev => [...prev, event.data]);
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

  const removeRecipe = useCallback((index) => {
    setRecipes(prev => prev.filter((_, i) => i !== index));
  }, []);

  return { events, recipes, loading, error, generate, reset, removeRecipe };
}
