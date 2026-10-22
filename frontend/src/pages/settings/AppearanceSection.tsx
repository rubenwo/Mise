import { useState } from 'react';

type Theme = 'light' | 'system' | 'dark';

const THEME_KEY = 'mise-theme';

export default function AppearanceSection() {
  const [theme, setTheme] = useState<Theme>(() => {
    try { return (localStorage.getItem(THEME_KEY) as Theme | null) || 'system'; } catch { return 'system'; }
  });

  const apply = (next: Theme) => {
    setTheme(next);
    try {
      if (next === 'system') {
        localStorage.removeItem(THEME_KEY);
        document.documentElement.removeAttribute('data-theme');
      } else {
        localStorage.setItem(THEME_KEY, next);
        document.documentElement.setAttribute('data-theme', next);
      }
    } catch {
      // Storage can be unavailable (private mode); the choice just won't persist.
    }
  };

  return (
    <div className="settings-section">
      <h3>Appearance</h3>
      <p className="settings-description">
        Choose how Mise looks. System follows your operating system preference.
      </p>
      <div className="theme-toggle" role="radiogroup" aria-label="Theme">
        <button
          role="radio"
          aria-checked={theme === 'light'}
          className={theme === 'light' ? 'active' : ''}
          onClick={() => apply('light')}
        >
          Light
        </button>
        <button
          role="radio"
          aria-checked={theme === 'system'}
          className={theme === 'system' ? 'active' : ''}
          onClick={() => apply('system')}
        >
          System
        </button>
        <button
          role="radio"
          aria-checked={theme === 'dark'}
          className={theme === 'dark' ? 'active' : ''}
          onClick={() => apply('dark')}
        >
          Dark
        </button>
      </div>
    </div>
  );
}
