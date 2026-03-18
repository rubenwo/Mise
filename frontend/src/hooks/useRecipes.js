import { useState, useEffect, useCallback } from 'react';
import { listRecipes, searchRecipes, deleteRecipe } from '../api/client';

export function useRecipes() {
  const [recipes, setRecipes] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const load = useCallback(async (limit = 20, offset = 0) => {
    setLoading(true);
    try {
      const data = await listRecipes(limit, offset);
      setRecipes(data.recipes || []);
      setTotal(data.total || 0);
      setError(null);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  const search = useCallback(async (params) => {
    setLoading(true);
    try {
      const data = await searchRecipes(params);
      setRecipes(data.recipes || []);
      setTotal(data.total || 0);
      setError(null);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  const remove = useCallback(async (id) => {
    try {
      await deleteRecipe(id);
      setRecipes(prev => prev.filter(r => r.id !== id));
      setTotal(prev => prev - 1);
    } catch (err) {
      setError(err.message);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return { recipes, total, loading, error, load, search, remove };
}
