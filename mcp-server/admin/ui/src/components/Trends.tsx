import { useState, useCallback } from 'preact/hooks';
import { usePolling } from '../hooks/usePolling';
import { fetchTrends, TrendsData } from '../api/client';

function directionBadge(dir: string) {
  const colors: Record<string, string> = {
    improving: '#4caf50',
    degrading: '#f44336',
    stable: '#ff9800',
    insufficient_data: '#666',
  };
  const symbols: Record<string, string> = {
    improving: '\u2193',
    degrading: '\u2191',
    stable: '\u2192',
    insufficient_data: '?',
  };
  return (
    <span style={{ color: colors[dir] || '#666', fontWeight: 'bold' }}>
      {symbols[dir] || '?'} {dir}
    </span>
  );
}

export function Trends() {
  const [scanTypeFilter, setScanTypeFilter] = useState('');

  const fetcher = useCallback(
    () => fetchTrends(scanTypeFilter || undefined),
    [scanTypeFilter]
  );
  const { data, loading, error } = usePolling<TrendsData>(fetcher, 10000);

  if (loading && !data) return <div class="loading">Loading trends...</div>;
  if (error) return <div class="error">Error: {error}</div>;
  if (!data) return null;

  // Get unique scan types for filter.
  const scanTypes = [...new Set(data.snapshots.map(s => s.scan_type))].sort();

  // Group snapshots by type for display.
  const byType = new Map<string, typeof data.snapshots>();
  for (const s of data.snapshots) {
    if (!byType.has(s.scan_type)) byType.set(s.scan_type, []);
    byType.get(s.scan_type)!.push(s);
  }

  return (
    <div class="trends">
      <h2>Scan Trends</h2>

      {scanTypes.length > 1 && (
        <div class="filter-bar">
          <label>Filter: </label>
          <select
            value={scanTypeFilter}
            onChange={e => setScanTypeFilter((e.target as HTMLSelectElement).value)}
          >
            <option value="">All scan types</option>
            {scanTypes.map(st => (
              <option key={st} value={st}>{st}</option>
            ))}
          </select>
        </div>
      )}

      {data.directions.length > 0 && (
        <div class="card-grid">
          {data.directions.map(d => (
            <div class="card" key={d.scan_type}>
              <div class="card-label">{d.scan_type}</div>
              <div class="card-value">{directionBadge(d.direction)}</div>
            </div>
          ))}
        </div>
      )}

      {[...byType.entries()].map(([type, snapshots]) => (
        <div class="section" key={type}>
          <h3>{type}</h3>
          <div class="sparkline">
            {snapshots.slice().reverse().map((s, i) => {
              const max = Math.max(...snapshots.map(x => x.findings_count), 1);
              const height = Math.max((s.findings_count / max) * 60, 4);
              return (
                <div
                  key={i}
                  class="spark-bar"
                  style={{ height: `${height}px` }}
                  title={`${s.findings_count} findings \u2014 ${new Date(s.created_at).toLocaleDateString()}`}
                />
              );
            })}
          </div>
          <table>
            <thead>
              <tr><th>Date</th><th>Findings</th></tr>
            </thead>
            <tbody>
              {snapshots.slice(0, 10).map((s, i) => (
                <tr key={i}>
                  <td>{new Date(s.created_at).toLocaleString()}</td>
                  <td>{s.findings_count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}

      {data.snapshots.length === 0 && (
        <div class="empty-state">No scan history available yet. Run /go to start building trend data.</div>
      )}
    </div>
  );
}
