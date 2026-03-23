import { useState, useEffect, useCallback } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import Layout from './components/Layout';
import AddRecipePage from './pages/AddRecipePage';
import InventoryPage from './pages/InventoryPage';
import LibraryPage from './pages/LibraryPage';
import PlanPage from './pages/PlanPage';
import PendingPage from './pages/PendingPage';
import RecipePage from './pages/RecipePage';
import SettingsPage from './pages/SettingsPage';
import { listPendingRecipes, scanIngredient } from './api/client';

let nextScanId = 1;

export default function App() {
  const [pendingCount, setPendingCount] = useState(0);
  const [pendingScans, setPendingScans] = useState([]);

  const refreshPendingCount = () => {
    listPendingRecipes()
      .then(data => setPendingCount((data || []).length))
      .catch(() => {});
  };

  useEffect(() => { refreshPendingCount(); }, []);

  const handleQueued = useCallback((file) => {
    const id = nextScanId++;
    const preview = URL.createObjectURL(file);
    setPendingScans(prev => [...prev, { id, status: 'processing', preview, result: null, error: null }]);

    scanIngredient(file)
      .then(async (res) => {
        if (!res.ok) {
          const err = await res.json().catch(() => ({ error: res.statusText }));
          throw new Error(err.error || 'Scan failed');
        }
        return res.json();
      })
      .then((scan) => {
        setPendingScans(prev => prev.map(s => s.id !== id ? s : {
          ...s,
          status: 'done',
          result: {
            name: scan.name || '',
            amount: scan.amount > 0 ? String(scan.amount) : '',
            unit: scan.unit || '',
            notes: scan.confident ? '' : '⚠ Low confidence — please verify',
          },
        }));
      })
      .catch((err) => {
        setPendingScans(prev => prev.map(s => s.id !== id ? s : { ...s, status: 'error', error: err.message }));
      });
  }, []);

  const handleDismiss = useCallback((scanId) => {
    setPendingScans(prev => prev.filter(s => s.id !== scanId));
  }, []);

  return (
    <BrowserRouter>
      <Layout pendingCount={pendingCount}>
        <Routes>
          <Route path="/" element={<PendingPage onCountChange={setPendingCount} />} />
          <Route path="/generate" element={<Navigate to="/recipe/new?mode=generate" replace />} />
          <Route path="/import" element={<Navigate to="/recipe/new?mode=import" replace />} />
          <Route path="/library" element={<LibraryPage />} />
          <Route path="/plans" element={<PlanPage />} />
          <Route path="/plans/:id" element={<PlanPage />} />
          <Route path="/recipe/new" element={<AddRecipePage />} />
          <Route path="/recipe/:id" element={<RecipePage />} />
          <Route path="/inventory" element={
            <InventoryPage
              pendingScans={pendingScans}
              onQueued={handleQueued}
              onDismiss={handleDismiss}
              onPendingAdded={handleDismiss}
            />
          } />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  );
}
