import { useState, useCallback } from 'preact/hooks';
import { usePolling } from '../hooks/usePolling';
import {
  fetchRenovate,
  RenovateResponse,
  RenovatePreference,
  RenovateRule,
  RenovateConfigScore,
} from '../api/client';

type Section = 'preferences' | 'rules' | 'scores';

function severityClass(sev: string): string {
  return `badge severity-${sev.toLowerCase()}`;
}

function scoreClass(score: number): string {
  if (score > 80) return 'score-good';
  if (score >= 60) return 'score-medium';
  return 'score-poor';
}

export function RenovateInsights() {
  const [activeSection, setActiveSection] = useState<Section>('preferences');
  const [categoryFilter, setCategoryFilter] = useState('');
  const [expandedPrefs, setExpandedPrefs] = useState<Set<number>>(new Set());
  const [expandedRules, setExpandedRules] = useState<Set<number>>(new Set());

  const fetcher = useCallback(
    () => fetchRenovate(categoryFilter || undefined),
    [categoryFilter]
  );

  const { data, loading, error } = usePolling<RenovateResponse>(fetcher, 30000);

  if (loading && !data) return <div class="loading">Loading Renovate insights...</div>;
  if (error) return <div class="error">Error: {error}</div>;

  const preferences = data?.preferences || [];
  const rules = data?.rules || [];
  const configScores = data?.config_scores || [];

  // Derive unique categories across preferences and rules
  const allCategories = [
    ...new Set([
      ...preferences.map((p) => p.category),
      ...rules.map((r) => r.category),
    ]),
  ].sort();

  // Sort preferences by frequency DESC
  const sortedPrefs = [...preferences].sort((a, b) => b.frequency - a.frequency);

  // Sort config scores by created_at DESC
  const sortedScores = [...configScores].sort(
    (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  );
  const latestScore = sortedScores.length > 0 ? sortedScores[0] : null;

  function togglePref(id: number) {
    setExpandedPrefs((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleRule(id: number) {
    setExpandedRules((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <div class="view-container">
      <h2>Renovate Insights</h2>

      <div class="section-tabs" role="tablist" aria-label="Renovate insight sections">
        <button
          role="tab"
          class={`section-tab ${activeSection === 'preferences' ? 'active' : ''}`}
          aria-selected={activeSection === 'preferences'}
          onClick={() => setActiveSection('preferences')}
        >
          Learned Preferences ({preferences.length})
        </button>
        <button
          role="tab"
          class={`section-tab ${activeSection === 'rules' ? 'active' : ''}`}
          aria-selected={activeSection === 'rules'}
          onClick={() => setActiveSection('rules')}
        >
          Best Practice Rules ({rules.length})
        </button>
        <button
          role="tab"
          class={`section-tab ${activeSection === 'scores' ? 'active' : ''}`}
          aria-selected={activeSection === 'scores'}
          onClick={() => setActiveSection('scores')}
        >
          Config Scores ({configScores.length})
        </button>
      </div>

      {/* Category filter - shared for preferences and rules sections */}
      {activeSection !== 'scores' && allCategories.length > 0 && (
        <div class="filter-bar">
          <select
            value={categoryFilter}
            onChange={(e) => setCategoryFilter((e.target as HTMLSelectElement).value)}
          >
            <option value="">All categories</option>
            {allCategories.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </div>
      )}

      {/* Section 1: Learned Preferences */}
      {activeSection === 'preferences' && (
        <div>
          {sortedPrefs.length === 0 ? (
            <div class="empty-state">No learned preferences yet.</div>
          ) : (
            <div class="table-wrapper">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>Category</th>
                    <th>Description</th>
                    <th>Frequency</th>
                    <th>Source</th>
                  </tr>
                </thead>
                <tbody>
                  {sortedPrefs.map((p) => (
                    <>
                      <tr
                        key={p.id}
                        tabIndex={0}
                        aria-expanded={expandedPrefs.has(p.id)}
                        onClick={() => togglePref(p.id)}
                        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); togglePref(p.id); }}}
                        style="cursor:pointer;"
                      >
                        <td>
                          <span class="finding-tag">{p.category}</span>
                        </td>
                        <td>{p.description}</td>
                        <td class="cell-number">{p.frequency}</td>
                        <td>{p.source}</td>
                      </tr>
                      {expandedPrefs.has(p.id) && (
                        <tr key={`${p.id}-detail`}>
                          <td colSpan={4}>
                            <div class="pattern-detail">
                              <div class="code-label">Don't (before)</div>
                              <div class="code-block">{p.dont_config}</div>
                              <div class="code-label">Do (after)</div>
                              <div class="code-block">{p.do_config}</div>
                            </div>
                          </td>
                        </tr>
                      )}
                    </>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* Section 2: Best Practice Rules */}
      {activeSection === 'rules' && (
        <div>
          {rules.length === 0 ? (
            <div class="empty-state">No best practice rules defined yet.</div>
          ) : (
            <div class="table-wrapper">
              <table class="data-table">
                <thead>
                  <tr>
                    <th>Rule ID</th>
                    <th>Title</th>
                    <th>Category</th>
                    <th>Severity</th>
                  </tr>
                </thead>
                <tbody>
                  {rules.map((r) => (
                    <>
                      <tr
                        key={r.id}
                        tabIndex={0}
                        aria-expanded={expandedRules.has(r.id)}
                        onClick={() => toggleRule(r.id)}
                        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleRule(r.id); }}}
                        style="cursor:pointer;"
                      >
                        <td class="cell-mono">{r.rule_id}</td>
                        <td>{r.title}</td>
                        <td>
                          <span class="finding-tag">{r.category}</span>
                        </td>
                        <td>
                          <span class={severityClass(r.severity)}>{r.severity}</span>
                        </td>
                      </tr>
                      {expandedRules.has(r.id) && (
                        <tr key={`${r.id}-detail`}>
                          <td colSpan={4}>
                            <div class="pattern-detail">
                              <p style="margin-bottom:0.5rem;color:#ccc;">{r.description}</p>
                              <div class="code-label">Don't (before)</div>
                              <div class="code-block">{r.dont_config}</div>
                              <div class="code-label">Do (after)</div>
                              <div class="code-block">{r.do_config}</div>
                            </div>
                          </td>
                        </tr>
                      )}
                    </>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* Section 3: Config Score History */}
      {activeSection === 'scores' && (
        <div>
          {sortedScores.length === 0 ? (
            <div class="empty-state">No config scores recorded yet</div>
          ) : (
            <>
              {latestScore && (
                <div class="card" style="text-align:center;margin-bottom:1rem;">
                  <div class="card-label">Latest Score</div>
                  <div class={`score-large ${scoreClass(latestScore.score)}`}>
                    {latestScore.score}
                  </div>
                  <div class="card-sub">
                    {latestScore.config_path} &mdash; {latestScore.findings_count} finding
                    {latestScore.findings_count !== 1 ? 's' : ''}
                  </div>
                  {sortedScores.length >= 2 && (
                    <div class="card-sub" style="margin-top:0.3rem;">
                      {latestScore.score > sortedScores[1].score
                        ? '\u2191 Improving'
                        : latestScore.score < sortedScores[1].score
                        ? '\u2193 Declining'
                        : '\u2192 Stable'}
                      {' from '}
                      {sortedScores[1].score}
                    </div>
                  )}
                </div>
              )}

              <div class="table-wrapper">
                <table class="data-table">
                  <thead>
                    <tr>
                      <th>Config Path</th>
                      <th>Score</th>
                      <th>Findings</th>
                      <th>Date</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sortedScores.map((s) => (
                      <tr key={s.id}>
                        <td class="cell-mono">{s.config_path}</td>
                        <td>
                          <span
                            style={`font-weight:bold;color:${
                              s.score > 80
                                ? '#4caf50'
                                : s.score >= 60
                                ? '#fdd835'
                                : '#f44336'
                            }`}
                          >
                            {s.score}
                          </span>
                        </td>
                        <td class="cell-number">{s.findings_count}</td>
                        <td>{new Date(s.created_at).toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}
