import { useState, useCallback } from 'preact/hooks';
import { usePolling } from '../hooks/usePolling';
import { fetchOWASP, OWASPResponse, OWASPCategory } from '../api/client';

const OWASP_CATEGORIES = [
  '',
  'A01',
  'A02',
  'A03',
  'A04',
  'A05',
  'A06',
  'A07',
  'A08',
  'A09',
  'A10',
] as const;

export function OWASPFindings() {
  const [categoryFilter, setCategoryFilter] = useState('');
  const [expandedCategories, setExpandedCategories] = useState<Set<string>>(new Set());

  const fetcher = useCallback(
    () => fetchOWASP(categoryFilter || undefined),
    [categoryFilter]
  );

  const { data, loading, error } = usePolling<OWASPResponse>(fetcher, 30000);

  if (loading && !data) return <div class="loading">Loading OWASP findings...</div>;
  if (error) return <div class="error">Error: {error}</div>;

  const categories = data?.categories || [];

  // Sort categories by code, sort findings within each by frequency DESC
  const sorted = [...categories].sort((a, b) => a.category.localeCompare(b.category));

  const maxCount = Math.max(...sorted.map((c) => c.finding_count), 1);

  function toggleCategory(cat: string) {
    setExpandedCategories((prev) => {
      const next = new Set(prev);
      if (next.has(cat)) next.delete(cat);
      else next.add(cat);
      return next;
    });
  }

  function categoryOpacity(cat: OWASPCategory): number {
    // More findings = more prominent, range 0.6 to 1.0
    return 0.6 + 0.4 * (cat.finding_count / maxCount);
  }

  return (
    <div class="view-container">
      <h2>OWASP Findings</h2>

      <div class="filter-bar">
        <select
          value={categoryFilter}
          onChange={(e) => setCategoryFilter((e.target as HTMLSelectElement).value)}
        >
          <option value="">All categories</option>
          {OWASP_CATEGORIES.filter(Boolean).map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
      </div>

      {sorted.length === 0 ? (
        <div class="empty-state">No OWASP findings recorded yet.</div>
      ) : (
        sorted.map((cat) => {
          const isExpanded = expandedCategories.has(cat.category);
          const sortedFindings = [...cat.findings].sort(
            (a, b) => b.frequency - a.frequency
          );
          return (
            <div class="owasp-category-card" key={cat.category}>
              <button
                class="owasp-category-header"
                style={{ opacity: categoryOpacity(cat) }}
                aria-expanded={isExpanded}
                onClick={() => toggleCategory(cat.category)}
              >
                <span>
                  <strong>{cat.category}</strong>
                  <span class="cve-count">
                    {cat.finding_count} finding{cat.finding_count !== 1 ? 's' : ''}
                  </span>
                </span>
                <span>{isExpanded ? '\u25B2' : '\u25BC'}</span>
              </button>
              {isExpanded && (
                <div class="owasp-category-body">
                  {sortedFindings.length === 0 ? (
                    <div class="empty-text">No findings in this category.</div>
                  ) : (
                    <table class="data-table">
                      <thead>
                        <tr>
                          <th>Finding</th>
                          <th>File Pattern</th>
                          <th>Fix Pattern</th>
                          <th>Frequency</th>
                        </tr>
                      </thead>
                      <tbody>
                        {sortedFindings.map((f) => (
                          <tr key={f.id}>
                            <td>{f.finding}</td>
                            <td class="cell-mono">{f.file_pattern}</td>
                            <td class="cell-mono">{f.fix_pattern}</td>
                            <td class="cell-number">{f.frequency}</td>
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
