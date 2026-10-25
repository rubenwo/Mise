import type { ReactNode } from 'react';
import { Link, NavLink } from 'react-router-dom';
import { BookOpen, CalendarDays, House, Inbox, Package, Plus, Settings, type LucideIcon } from 'lucide-react';

interface LayoutProps {
  children: ReactNode;
  pendingCount?: number;
}

interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
}

const NAV_ITEMS: NavItem[] = [
  { to: '/', label: 'Home', icon: House },
  { to: '/plans', label: 'Plans', icon: CalendarDays },
  { to: '/library', label: 'Library', icon: BookOpen },
  { to: '/inventory', label: 'Pantry', icon: Package },
];

function Logo() {
  return (
    <Link to="/" className="logo">
      <svg width="22" height="22" viewBox="0 0 22 22" aria-hidden="true">
        <circle cx="11" cy="11" r="10" fill="var(--tomato)" />
        <path d="M11 3c1.5 2 1.5 4 0 6" stroke="#fff" strokeWidth="1.6" fill="none" strokeLinecap="round" />
      </svg>
      Mise
    </Link>
  );
}

function InboxLink({ pendingCount, className }: { pendingCount: number; className: string }) {
  const label = pendingCount > 0 ? `${pendingCount} recipes waiting for review` : 'Recipes to review';
  return (
    <NavLink to="/pending" className={className} aria-label={label} title={label}>
      <Inbox size={22} strokeWidth={1.8} aria-hidden="true" />
      {pendingCount > 0 && <span className="nav-badge">{pendingCount}</span>}
    </NavLink>
  );
}

export default function Layout({ children, pendingCount = 0 }: LayoutProps) {
  return (
    <div className="app">
      <header className="header">
        <nav className="header-content" aria-label="Main">
          <Logo />
          <div className="nav">
            {NAV_ITEMS.map(item => (
              <NavLink key={item.to} to={item.to} end={item.to === '/'} className="nav-link">{item.label}</NavLink>
            ))}
          </div>
          <div className="nav-actions">
            <InboxLink pendingCount={pendingCount} className="nav-icon nav-desktop-only" />
            <NavLink to="/settings" className="nav-icon" aria-label="Settings" title="Settings">
              <Settings size={21} strokeWidth={1.8} aria-hidden="true" />
            </NavLink>
            <Link to="/recipe/new" className="btn btn-primary nav-new">
              <Plus size={16} strokeWidth={2.2} aria-hidden="true" />
              <span className="nav-new-label">New recipe</span>
            </Link>
          </div>
        </nav>
      </header>
      <main className="main">{children}</main>
      <nav className="tabbar" aria-label="Main">
        {NAV_ITEMS.map(({ to, label, icon: Icon }) => (
          <NavLink key={to} to={to} end={to === '/'} className="tabbar-link">
            <Icon size={22} strokeWidth={1.8} aria-hidden="true" />
            {label}
          </NavLink>
        ))}
        <NavLink to="/pending" className="tabbar-link">
          <span className="tabbar-icon">
            <Inbox size={22} strokeWidth={1.8} aria-hidden="true" />
            {pendingCount > 0 && <span className="nav-badge">{pendingCount}</span>}
          </span>
          Review
        </NavLink>
      </nav>
    </div>
  );
}
