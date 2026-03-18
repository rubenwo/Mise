import { Link, useLocation } from 'react-router-dom';

export default function Layout({ children }) {
  const location = useLocation();

  return (
    <div className="app">
      <header className="header">
        <div className="header-content">
          <Link to="/" className="logo">Recipe Generator</Link>
          <nav className="nav">
            <Link to="/" className={location.pathname === '/' ? 'active' : ''}>Generate</Link>
            <Link to="/library" className={location.pathname === '/library' ? 'active' : ''}>Library</Link>
          </nav>
        </div>
      </header>
      <main className="main">{children}</main>
    </div>
  );
}
