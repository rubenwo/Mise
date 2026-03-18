import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import GeneratePage from './pages/GeneratePage';
import ImportPage from './pages/ImportPage';
import LibraryPage from './pages/LibraryPage';
import PlanPage from './pages/PlanPage';
import RecipePage from './pages/RecipePage';

export default function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<GeneratePage />} />
          <Route path="/import" element={<ImportPage />} />
          <Route path="/library" element={<LibraryPage />} />
          <Route path="/plans" element={<PlanPage />} />
          <Route path="/plans/:id" element={<PlanPage />} />
          <Route path="/recipe/:id" element={<RecipePage />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  );
}
