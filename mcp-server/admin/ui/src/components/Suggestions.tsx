import { useState, useEffect } from 'preact/hooks';
import { fetchSuggestions, SuggestionEntry } from '../api/client';

function humanizeType(type: string): string {
  const map: Record<string, string> = {
    empty_fix: 'Empty Fix Suggestion',
    low_frequency: 'Low Frequency',
    duplicate_code: 'Duplicate Code',
    stale_pattern: 'Stale Pattern',
    missing_glob: 'Missing File Glob',
  };
  return map[type] || type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

export function Suggestions() {
  const [data, setData] = useState<SuggestionEntry[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    try {
      const items = await fetchSuggestions();
      setData(items);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  return (
    <div class="view-container">
      <h2>Quality Suggestions</h2>

      <div style={{ marginBottom: '1rem' }}>
        <button class="btn btn-sm" onClick={load} disabled={loading}>
          {loading ? 'Refreshing...' : 'Refresh'}
        </button>
      </div>

      {loading && !data && <p class="loading-text">Loading suggestions...</p>}
      {error && <p class="error-text">Error: {error}</p>}

      {data && data.length === 0 && (
        <div class="healthy-state">Knowledge base looks healthy -- no suggestions at this time.</div>
      )}

      {data && data.length > 0 && (
        <div class="suggestions-grid">
          {data.map((s, i) => (
            <div class="suggestion-card" key={i}>
              <div class="suggestion-type">{humanizeType(s.type)}</div>
              <div class="suggestion-desc">{s.description}</div>
              <div class="suggestion-meta">
                <span>{s.pattern_ids.length} pattern{s.pattern_ids.length !== 1 ? 's' : ''} affected</span>
                <span class="suggestion-action">{s.action}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
