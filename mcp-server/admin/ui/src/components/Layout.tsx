import { ComponentChildren } from 'preact';
import { Sidebar } from './Sidebar';

interface NavItem {
  path: string;
  label: string;
}

interface LayoutProps {
  currentPath: string;
  navItems: NavItem[];
  children: ComponentChildren;
}

export function Layout({ currentPath, navItems, children }: LayoutProps) {
  return (
    <div class="layout">
      <Sidebar currentPath={currentPath} navItems={navItems} />
      <main class="content">{children}</main>
    </div>
  );
}
