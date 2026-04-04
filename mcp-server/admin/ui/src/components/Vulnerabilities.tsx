import { useState, useCallback, useEffect } from 'preact/hooks';
import { usePolling } from '../hooks/usePolling';
import { fetchVulnerabilities, fetchPrefetchStatus, VulnResponse, VulnModule, PrefetchStatus } from '../api/client';

const SEVERITIES = ['', 'LOW', 'MEDIUM', 'HIGH', 'CRITICAL'] as const;

function severityClass(sev: string): string {
  return `badge severity-${sev.toLowerCase()}`;
}

function sourceLabel(source: string): string {
  if (source === 'nvd') return 'NVD';
  if (source === 'go-vuln') return 'Go Vuln DB';
  return source || 'Go Vuln DB';
}

function decisionBadge(mod: VulnModule) {
  if (!mod.decision) {
    return <span class="badge decision-unreviewed">unreviewed</span>;
  }
  const d = mod.decision.decision.toLowerCase();
  let cls = 'decision-ignore';
  if (d === 'upgrade') cls = 'decision-upgrade';
  else if (d.includes('accept')) cls = 'decision-accept';
  return <span class={`badge ${cls}`}>{mod.decision.decision}</span>;
}

function PrefetchBanner() {
  const [status, setStatus] = useState<PrefetchStatus | null>(null);

  useEffect(() => {
    let active = true;
    const poll = async () => {
      try {
        const s = await fetchPrefetchStatus();
        if (active) setStatus(s);
      } catch {
        // ignore
      }
    };
    poll();
    const id = setInterval(poll, 2000);
    return () => { active = false; clearInterval(id); };
  }, []);

  if (!status || status.phase === 'idle' || status.phase === 'done') return null;

  const isError = status.phase === 'error';
  const pct = status.total > 0 ? Math.round((status.progress / status.total) * 100) : 0;

  return (
    <div class={`prefetch-banner ${isError ? 'prefetch-error' : ''}`}>
      {isError ? (
        <span>Prefetch error: {status.error}</span>
      ) : (
        <>
          <span class="prefetch-source">
            Fetching from <strong>{status.source}</strong>
          </span>
          <div class="prefetch-progress-bar">
            <div class="prefetch-progress-fill" style={{ width: `${pct}%` }} />
          </div>
          <span class="prefetch-pct">
            {status.progress}/{status.total} CVEs ({pct}%)
          </span>
        </>
      )}
    </div>
  );
}

export function Vulnerabilities() {
  const [severityFilter, setSeverityFilter] = useState('');
  const [moduleSearch, setModuleSearch] = useState('');
  const [expandedModules, setExpandedModules] = useState<Set<string>>(new Set());

  const fetcher = useCallback(
    () =>
      fetchVulnerabilities({
        severity: severityFilter || undefined,
        module: moduleSearch || undefined,
      }),
    [severityFilter, moduleSearch]
  );

  const { data, loading, error } = usePolling<VulnResponse>(fetcher, 30000);

  if (loading && !data) return <div class="loading">Loading vulnerabilities...</div>;
  if (error) return <div class="error">Error: {error}</div>;

  const modules = data?.modules || [];

  // Collect source stats.
  const sourceCounts: Record<string, number> = {};
  for (const mod of modules) {
    for (const v of mod.vulnerabilities) {
      const src = v.source || 'go-vuln';
      sourceCounts[src] = (sourceCounts[src] || 0) + 1;
    }
  }
  const totalCVEs = Object.values(sourceCounts).reduce((a, b) => a + b, 0);

  function toggleModule(mod: string) {
    setExpandedModules((prev) => {
      const next = new Set(prev);
      if (next.has(mod)) next.delete(mod);
      else next.add(mod);
      return next;
    });
  }

  return (
    <div class="view-container">
      <h2>Vulnerabilities</h2>

      <PrefetchBanner />

      {/* Source summary */}
      {totalCVEs > 0 && (
        <div class="source-summary">
          <span class="source-total">{totalCVEs} CVEs from:</span>
          {Object.entries(sourceCounts).map(([src, count]) => (
            <span key={src} class={`badge source-badge source-${src}`}>
              {sourceLabel(src)} ({count})
            </span>
          ))}
        </div>
      )}

      <div class="filter-bar">
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
          placeholder="Search modules..."
          value={moduleSearch}
          onInput={(e) => setModuleSearch((e.target as HTMLInputElement).value)}
          style="background:rgba(255,255,255,0.08);color:#ddd;border:1px solid rgba(255,255,255,0.15);border-radius:4px;padding:0.4rem 0.6rem;font-size:0.9rem;"
        />
      </div>

      {modules.length === 0 ? (
        <div class="empty-state">No vulnerability data available.</div>
      ) : (
        modules.map((mod) => {
          const isExpanded = expandedModules.has(mod.module);
          const cveCount = mod.decision?.cve_count ?? mod.vulnerabilities.length;
          return (
            <div class="module-group" key={mod.module}>
              <button
                class="collapsible-header"
                aria-expanded={isExpanded}
                onClick={() => toggleModule(mod.module)}
              >
                <span>
                  <span class="module-name">{mod.module}</span>
                  <span class="cve-count">{cveCount} CVE{cveCount !== 1 ? 's' : ''}</span>
                </span>
                <span style="display:flex;align-items:center;gap:0.5rem;">
                  {decisionBadge(mod)}
                  <span>{isExpanded ? '\u25B2' : '\u25BC'}</span>
                </span>
              </button>
              {isExpanded && (
                <div class="collapsible-body">
                  {mod.decision && (
                    <div style="margin-bottom:0.5rem;font-size:0.85rem;color:#999;">
                      <strong>Reason:</strong> {mod.decision.reason}
                      <span style="margin-left:1rem;color:#777;">
                        Checked: {new Date(mod.decision.checked_at).toLocaleDateString()}
                      </span>
                    </div>
                  )}
                  {mod.vulnerabilities.length === 0 ? (
                    <div class="empty-text">No individual CVEs listed.</div>
                  ) : (
                    <table class="data-table">
                      <thead>
                        <tr>
                          <th>CVE ID</th>
                          <th>Severity</th>
                          <th>Source</th>
                          <th>Affected Versions</th>
                          <th>Fixed Version</th>
                          <th>Description</th>
                        </tr>
                      </thead>
                      <tbody>
                        {mod.vulnerabilities.map((v) => (
                          <tr key={v.cve_id}>
                            <td class="cell-mono">{v.cve_id}</td>
                            <td>
                              <span class={severityClass(v.severity)}>{v.severity}</span>
                            </td>
                            <td>
                              <span class={`badge source-badge source-${v.source || 'go-vuln'}`}>
                                {sourceLabel(v.source)}
                              </span>
                            </td>
                            <td class="cell-mono">{v.affected_versions}</td>
                            <td class="cell-mono">{v.fixed_version || <span class="cell-dash">-</span>}</td>
                            <td>{v.description}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  )}
                </div>
              )}
            </div>
          );
        })
      )}
    </div>
  );
}
