import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import GeneratePage from './pages/GeneratePage';
import LibraryPage from './pages/LibraryPage';
import RecipePage from './pages/RecipePage';

export default function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<GeneratePage />} />
          <Route path="/library" element={<LibraryPage />} />
          <Route path="/recipe/:id" element={<RecipePage />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  );
}
