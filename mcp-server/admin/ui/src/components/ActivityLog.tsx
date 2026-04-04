import { useCallback } from 'preact/hooks';
import { fetchActivity, MCPRequest } from '../api/client';
import { usePolling } from '../hooks/usePolling';

function relativeTime(iso: string): string {
  const now = Date.now();
  const then = new Date(iso).getTime();
  const diffMs = now - then;

  if (isNaN(diffMs) || diffMs < 0) return iso;

  const seconds = Math.floor(diffMs / 1000);
  if (seconds < 60) return `${seconds}s ago`;

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;

  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function ActivityLog() {
  const fetchFn = useCallback(() => fetchActivity(100, 0), []);
  const { data, loading, error } = usePolling<MCPRequest[]>(fetchFn, 5000);

  if (loading && !data) {
    return <div class="view-container"><p class="loading-text">Loading...</p></div>;
  }

  if (error) {
    return (
      <div class="view-container">
        <h2>Activity Log</h2>
        <p class="error-text">Error: {error}</p>
      </div>
    );
  }

  const items = data ?? [];

  return (
    <div class="view-container">
      <h2>Activity Log</h2>
      {items.length === 0 ? (
        <p class="empty-text">No activity recorded yet</p>
      ) : (
        <div class="table-wrapper">
          <table class="data-table">
            <thead>
              <tr>
                <th>Time</th>
                <th>Tool</th>
                <th>Agent</th>
                <th>Params</th>
                <th>Duration (ms)</th>
                <th>Error</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id}>
                  <td class="cell-mono">{relativeTime(item.created_at)}</td>
                  <td class="cell-mono">{item.tool_name}</td>
                  <td>{item.agent}</td>
                  <td class="cell-mono cell-params">{item.params_summary}</td>
                  <td class="cell-number">{item.duration_ms}</td>
                  <td class={item.error ? 'cell-error' : 'cell-dash'}>
                    {item.error || '\u2014'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
