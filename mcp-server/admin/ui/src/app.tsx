import { useState, useEffect } from 'preact/hooks';
import { Layout } from './components/Layout';
import { ActivityLog } from './components/ActivityLog';
import { Dashboard } from './components/Dashboard';
import { Trends } from './components/Trends';
import { Patterns } from './components/Patterns';
import { Suggestions } from './components/Suggestions';
import { Placeholder } from './components/Placeholder';
import { SessionFindings } from './components/SessionFindings';
import { OWASPFindings } from './components/OWASPFindings';
import { Vulnerabilities } from './components/Vulnerabilities';
import { RenovateInsights } from './components/RenovateInsights';

function getHash(): string {
  const hash = window.location.hash.replace(/^#/, '') || '/';
  return hash;
}

const NAV_ITEMS = [
  { path: '/', label: 'Dashboard' },
  { path: '/patterns', label: 'Patterns' },
  { path: '/trends', label: 'Scan Trends' },
  { path: '/sessions', label: 'Session Findings' },
  { path: '/owasp', label: 'OWASP Findings' },
  { path: '/vulnerabilities', label: 'Vulnerabilities' },
  { path: '/renovate', label: 'Renovate' },
  { path: '/suggestions', label: 'Quality' },
  { path: '/activity', label: 'Activity Log' },
];

function renderView(path: string) {
  switch (path) {
    case '/':
      return <Dashboard />;
    case '/trends':
      return <Trends />;
    case '/patterns':
      return <Patterns />;
    case '/sessions':
      return <SessionFindings />;
    case '/owasp':
      return <OWASPFindings />;
    case '/vulnerabilities':
      return <Vulnerabilities />;
    case '/renovate':
      return <RenovateInsights />;
    case '/suggestions':
      return <Suggestions />;
    case '/activity':
      return <ActivityLog />;
    default: {
      const item = NAV_ITEMS.find((n) => n.path === path);
      const name = item ? item.label : 'Not Found';
      return <Placeholder name={name} />;
    }
  }
}

export function App() {
  const [currentPath, setCurrentPath] = useState(getHash());

  useEffect(() => {
    const onHashChange = () => setCurrentPath(getHash());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  return (
    <Layout currentPath={currentPath} navItems={NAV_ITEMS}>
      {renderView(currentPath)}
    </Layout>
  );
}
