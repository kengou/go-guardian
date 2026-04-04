import { useState, useCallback } from 'preact/hooks';
import { usePolling } from '../hooks/usePolling';
import { fetchSessionFindings, SessionFinding } from '../api/client';

const SEVERITIES = ['', 'LOW', 'MEDIUM', 'HIGH', 'CRITICAL'] as const;

function severityClass(sev: string): string {
  return `badge severity-${sev.toLowerCase()}`;
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  if (isNaN(diff) || diff < 0) return dateStr;
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function SessionFindings() {
  const [agentFilter, setAgentFilter] = useState('');
  const [severityFilter, setSeverityFilter] = useState('');
  const [typeFilter, setTypeFilter] = useState('');

  const fetcher = useCallback(
    () =>
      fetchSessionFindings({
        agent: agentFilter || undefined,
        severity: severityFilter || undefined,
        finding_type: typeFilter || undefined,
      }),
    [agentFilter, severityFilter, typeFilter]
  );

  const { data, loading, error } = usePolling<SessionFinding[]>(fetcher, 10000);

  if (loading && !data) return <div class="loading">Loading session findings...</div>;
  if (error) return <div class="error">Error: {error}</div>;

  const findings = data || [];

  // Derive unique agents for dropdown
  const agents = [...new Set(findings.map((f) => f.agent))].sort();

  return (
    <div class="view-container">
      <h2>Session Findings</h2>

      <div class="filter-bar">
        <select
          value={agentFilter}
          onChange={(e) => setAgentFilter((e.target as HTMLSelectElement).value)}
        >
          <option value="">All agents</option>
          {agents.map((a) => (
            <option key={a} value={a}>
              {a}
            </option>
          ))}
        </select>

        <select
          value={severityFilter}
          onChange={(e) => setSeverityFilter((e.target as HTMLSelectElement).value)}
        >
          <option value="">All severities</option>
          {SEVERITIES.filter(Boolean).map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>

        <input
          type="text"
          placeholder="Filter by finding type..."
          value={typeFilter}
          onInput={(e) => setTypeFilter((e.target as HTMLInputElement).value)}
          style="background:rgba(255,255,255,0.08);color:#ddd;border:1px solid rgba(255,255,255,0.15);border-radius:4px;padding:0.4rem 0.6rem;font-size:0.9rem;"
        />
      </div>

      {findings.length === 0 ? (
        <div class="empty-state">No session findings</div>
      ) : (
        <div class="table-wrapper">
          <table class="data-table">
            <thead>
              <tr>
                <th>Agent</th>
                <th>Severity</th>
                <th>Finding Type</th>
                <th>File Path</th>
                <th>Description</th>
                <th>Time</th>
              </tr>
            </thead>
            <tbody>
              {findings.map((f) => (
                <tr key={f.id}>
                  <td>{f.agent}</td>
                  <td>
                    <span class={severityClass(f.severity)}>{f.severity}</span>
                  </td>
                  <td>
                    <span class="finding-tag">{f.finding_type}</span>
                  </td>
                  <td class="cell-mono">{f.file_path}</td>
                  <td>{f.description}</td>
                  <td title={f.created_at}>{timeAgo(f.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
