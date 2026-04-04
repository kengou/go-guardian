interface NavItem {
  path: string;
  label: string;
}

interface SidebarProps {
  currentPath: string;
  navItems: NavItem[];
}

export function Sidebar({ currentPath, navItems }: SidebarProps) {
  return (
    <aside class="sidebar">
      <div class="sidebar-header">
        <h1 class="sidebar-title">go-guardian</h1>
        <span class="sidebar-subtitle">admin</span>
      </div>
      <nav class="sidebar-nav">
        <ul>
          {navItems.map((item) => (
            <li key={item.path}>
              <a
                href={`#${item.path}`}
                class={`nav-link ${currentPath === item.path ? 'active' : ''}`}
              >
                {item.label}
              </a>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  );
}
