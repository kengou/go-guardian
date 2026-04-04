import { usePolling } from '../hooks/usePolling';
import { fetchDashboard, DashboardData } from '../api/client';

function directionBadge(dir: string) {
  const colors: Record<string, string> = {
    improving: '#4caf50',
    degrading: '#f44336',
    stable: '#ff9800',
    insufficient_data: '#666',
  };
  const labels: Record<string, string> = {
    improving: '\u2193 Improving',
    degrading: '\u2191 Degrading',
    stable: '\u2192 Stable',
    insufficient_data: '? No data',
  };
  const color = colors[dir] || '#666';
  const label = labels[dir] || dir;
  return <span style={{ color, fontWeight: 'bold' }}>{label}</span>;
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function Dashboard() {
  const { data, loading, error } = usePolling<DashboardData>(fetchDashboard, 10000);

  if (loading && !data) return <div class="loading">Loading dashboard...</div>;
  if (error) return <div class="error">Error: {error}</div>;
  if (!data) return null;

  const owaspEntries = Object.entries(data.owasp_counts).sort(([a], [b]) => a.localeCompare(b));

  return (
    <div class="dashboard">
      <h2>Dashboard</h2>

      <div class="card-grid">
        <div class="card">
          <div class="card-label">Lint Patterns</div>
          <div class="card-value">{data.total_patterns}</div>
          <div class="card-sub">{data.recent_learning_count} learned this week</div>
        </div>

        <div class="card">
          <div class="card-label">Anti-Patterns</div>
          <div class="card-value">{data.total_anti_patterns}</div>
        </div>

        {data.session_info && (
          <div class="card">
            <div class="card-label">Session Findings</div>
            <div class="card-value">{data.session_info.finding_count}</div>
            <div class="card-sub" title={data.session_info.session_id}>
              {data.session_info.session_id.slice(0, 8)}...
            </div>
          </div>
        )}
      </div>

      {data.trend_summary.length > 0 && (
        <div class="section">
          <h3>Trend Summary</h3>
          <div class="trend-list">
            {data.trend_summary.map(t => (
              <div class="trend-item" key={t.scan_type}>
                <span class="trend-type">{t.scan_type}</span>
                {directionBadge(t.direction)}
              </div>
            ))}
          </div>
        </div>
      )}

      {data.recent_scans.length > 0 && (
        <div class="section">
          <h3>Recent Scans</h3>
          <table>
            <thead>
              <tr><th>Type</th><th>Last Run</th><th>Findings</th></tr>
            </thead>
            <tbody>
              {data.recent_scans.map(s => (
                <tr key={s.scan_type}>
                  <td>{s.scan_type}</td>
                  <td>{timeAgo(s.last_run)}</td>
                  <td>{s.findings_count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {owaspEntries.length > 0 && (
        <div class="section">
          <h3>OWASP Posture</h3>
          <div class="owasp-grid">
            {owaspEntries.map(([cat, count]) => (
              <div class="owasp-item" key={cat}>
                <span class="owasp-cat">{cat}</span>
                <span class="owasp-count">{count}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {data.total_patterns === 0 && data.total_anti_patterns === 0 && (
        <div class="empty-state">No patterns learned yet. Run /go to start building your knowledge base.</div>
      )}
    </div>
  );
}
