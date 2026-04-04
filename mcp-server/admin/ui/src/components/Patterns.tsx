import { useState, useEffect, useRef } from 'preact/hooks';
import {
  fetchPatterns,
  fetchPatternDetail,
  updatePattern,
  deletePattern,
  restorePattern,
  PatternEntry,
  PatternsResponse,
  HistoryEntry,
} from '../api/client';

const PER_PAGE = 25;

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  if (isNaN(diff) || diff < 0) return iso;
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

interface EditState {
  dont_code: string;
  do_code: string;
  rule: string;
  file_glob: string;
}

export function Patterns() {
  // Filters
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [sourceFilter, setSourceFilter] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [sortBy, setSortBy] = useState('');
  const [showDeleted, setShowDeleted] = useState(false);
  const [page, setPage] = useState(1);

  // Data
  const [data, setData] = useState<PatternsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Expanded row
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [detail, setDetail] = useState<{ pattern: PatternEntry; history: HistoryEntry[] } | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  // Edit mode
  const [editing, setEditing] = useState(false);
  const [editState, setEditState] = useState<EditState>({ dont_code: '', do_code: '', rule: '', file_glob: '' });
  const [saving, setSaving] = useState(false);

  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Debounce search
  useEffect(() => {
    if (debounceTimer.current) clearTimeout(debounceTimer.current);
    debounceTimer.current = setTimeout(() => {
      setDebouncedSearch(search);
      setPage(1);
    }, 300);
    return () => {
      if (debounceTimer.current) clearTimeout(debounceTimer.current);
    };
  }, [search]);

  // Fetch patterns
  const loadPatterns = async () => {
    setLoading(true);
    try {
      const resp = await fetchPatterns({
        search: debouncedSearch || undefined,
        source: sourceFilter || undefined,
        type: typeFilter || undefined,
        sort: sortBy || undefined,
        include_deleted: showDeleted || undefined,
        page,
        per_page: PER_PAGE,
      });
      setData(resp);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadPatterns();
  }, [debouncedSearch, sourceFilter, typeFilter, sortBy, showDeleted, page]);

  // Expand a row to show detail
  const toggleRow = async (entry: PatternEntry) => {
    if (expandedId === entry.id) {
      setExpandedId(null);
      setDetail(null);
      setEditing(false);
      return;
    }
    setExpandedId(entry.id);
    setEditing(false);
    setDetailLoading(true);
    try {
      const d = await fetchPatternDetail(entry.id, entry.type);
      setDetail(d);
    } catch {
      setDetail(null);
    } finally {
      setDetailLoading(false);
    }
  };

  const handleDelete = async (entry: PatternEntry) => {
    if (!window.confirm(`Delete pattern #${entry.id}? This soft-deletes the pattern.`)) return;
    try {
      await deletePattern(entry.id, entry.type);
      setExpandedId(null);
      setDetail(null);
      await loadPatterns();
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Delete failed');
    }
  };

  const handleRestore = async (entry: PatternEntry) => {
    try {
      await restorePattern(entry.id, entry.type);
      await loadPatterns();
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Restore failed');
    }
  };

  const startEdit = (p: PatternEntry) => {
    setEditing(true);
    setEditState({
      dont_code: p.dont_code,
      do_code: p.do_code,
      rule: p.rule || '',
      file_glob: p.file_glob || '',
    });
  };

  const handleSave = async (entry: PatternEntry) => {
    setSaving(true);
    try {
      await updatePattern(entry.id, {
        type: entry.type,
        dont_code: editState.dont_code,
        do_code: editState.do_code,
        rule: editState.rule || undefined,
        file_glob: editState.file_glob || undefined,
      });
      setEditing(false);
      // Refresh detail
      const d = await fetchPatternDetail(entry.id, entry.type);
      setDetail(d);
      await loadPatterns();
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  const totalPages = data ? Math.max(1, Math.ceil(data.total / PER_PAGE)) : 1;

  return (
    <div class="view-container">
      <h2>Patterns</h2>

      {/* Toolbar */}
      <div class="patterns-toolbar">
        <input
          type="text"
          placeholder="Search patterns..."
          value={search}
          onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
        />
        <select value={sourceFilter} onChange={(e) => { setSourceFilter((e.target as HTMLSelectElement).value); setPage(1); }}>
          <option value="">All sources</option>
          <option value="lint_fix">Lint Fix</option>
          <option value="manual">Manual</option>
          <option value="review">Review</option>
          <option value="curated">Curated</option>
        </select>
        <select value={typeFilter} onChange={(e) => { setTypeFilter((e.target as HTMLSelectElement).value); setPage(1); }}>
          <option value="">All types</option>
          <option value="lint">Lint</option>
          <option value="anti">Anti-pattern</option>
        </select>
        <select value={sortBy} onChange={(e) => { setSortBy((e.target as HTMLSelectElement).value); setPage(1); }}>
          <option value="">Default sort</option>
          <option value="frequency">Frequency</option>
          <option value="last_seen">Last Seen</option>
          <option value="created_at">Created</option>
        </select>
        <label>
          <input
            type="checkbox"
            checked={showDeleted}
            onChange={(e) => { setShowDeleted((e.target as HTMLInputElement).checked); setPage(1); }}
          />
          Show deleted
        </label>
      </div>

      {/* Loading / Error */}
      {loading && !data && <p class="loading-text">Loading patterns...</p>}
      {error && <p class="error-text">Error: {error}</p>}

      {data && data.items.length === 0 && (
        <p class="empty-text">No patterns found</p>
      )}

      {data && data.items.length > 0 && (
        <>
          <div class="table-wrapper">
            <table class="data-table">
              <thead>
                <tr>
                  <th>Type</th>
                  <th>Rule / ID</th>
                  <th>File Glob / Category</th>
                  <th>Frequency</th>
                  <th>Source</th>
                  <th>Last Seen</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((entry) => {
                  const isExpanded = expandedId === entry.id;
                  const isDeleted = entry.deleted_at !== null;
                  return (
                    <>
                      <tr
                        key={entry.id}
                        class={isDeleted ? 'deleted-row' : ''}
                        tabIndex={0}
                        aria-expanded={isExpanded}
                        style={{ cursor: 'pointer' }}
                        onClick={() => toggleRow(entry)}
                        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleRow(entry); }}}
                      >
                        <td>
                          <span class={`badge badge-${entry.type}`}>{entry.type}</span>
                          {isDeleted && <span class="deleted-badge">deleted</span>}
                        </td>
                        <td class="cell-mono">
                          {entry.type === 'lint' ? (entry.rule || '\u2014') : (entry.pattern_id || '\u2014')}
                        </td>
                        <td class="cell-mono">
                          {entry.type === 'lint' ? (entry.file_glob || '\u2014') : (entry.category || '\u2014')}
                        </td>
                        <td class="cell-number">
                          {entry.type === 'lint' ? (entry.frequency ?? '\u2014') : '\u2014'}
                        </td>
                        <td>{entry.source}</td>
                        <td class="cell-mono">
                          {entry.type === 'lint' && entry.last_seen ? relativeTime(entry.last_seen) : '\u2014'}
                        </td>
                        <td class="actions-cell" onClick={(e) => e.stopPropagation()}>
                          {isDeleted ? (
                            showDeleted && (
                              <button class="btn btn-sm btn-primary" onClick={() => handleRestore(entry)}>
                                Restore
                              </button>
                            )
                          ) : (
                            <button class="btn btn-sm btn-danger" onClick={() => handleDelete(entry)}>
                              Delete
                            </button>
                          )}
                        </td>
                      </tr>

                      {isExpanded && (
                        <tr key={`${entry.id}-detail`}>
                          <td colSpan={7} style={{ padding: 0 }}>
                            <div class="pattern-detail">
                              {detailLoading && <p class="loading-text">Loading detail...</p>}

                              {!detailLoading && detail && (
                                <>
                                  <div class="code-label">Don't (bad pattern)</div>
                                  <div class="code-block">{detail.pattern.dont_code}</div>

                                  <div class="code-label">Do (correct pattern)</div>
                                  <div class="code-block">{detail.pattern.do_code}</div>

                                  {detail.pattern.description && (
                                    <>
                                      <div class="code-label">Description</div>
                                      <p style={{ color: '#bbb', fontSize: '0.85rem', margin: '0.3rem 0' }}>
                                        {detail.pattern.description}
                                      </p>
                                    </>
                                  )}

                                  {/* Edit form — lint only */}
                                  {detail.pattern.type === 'lint' && !editing && !isDeleted && (
                                    <div class="btn-group">
                                      <button class="btn btn-primary btn-sm" onClick={() => startEdit(detail.pattern)}>
                                        Edit
                                      </button>
                                    </div>
                                  )}

                                  {editing && detail.pattern.type === 'lint' && (
                                    <div class="edit-form">
                                      <div class="form-group">
                                        <label>Rule</label>
                                        <input
                                          type="text"
                                          value={editState.rule}
                                          onInput={(e) => setEditState({ ...editState, rule: (e.target as HTMLInputElement).value })}
                                        />
                                      </div>
                                      <div class="form-group">
                                        <label>File Glob</label>
                                        <input
                                          type="text"
                                          value={editState.file_glob}
                                          onInput={(e) => setEditState({ ...editState, file_glob: (e.target as HTMLInputElement).value })}
                                        />
                                      </div>
                                      <div class="form-group">
                                        <label>Don't Code</label>
                                        <textarea
                                          value={editState.dont_code}
                                          onInput={(e) => setEditState({ ...editState, dont_code: (e.target as HTMLTextAreaElement).value })}
                                        />
                                      </div>
                                      <div class="form-group">
                                        <label>Do Code</label>
                                        <textarea
                                          value={editState.do_code}
                                          onInput={(e) => setEditState({ ...editState, do_code: (e.target as HTMLTextAreaElement).value })}
                                        />
                                      </div>
                                      <div class="btn-group">
                                        <button
                                          class="btn btn-primary btn-sm"
                                          disabled={saving}
                                          onClick={() => handleSave(detail.pattern)}
                                        >
                                          {saving ? 'Saving...' : 'Save'}
                                        </button>
                                        <button class="btn btn-sm" onClick={() => setEditing(false)}>
                                          Cancel
                                        </button>
                                      </div>
                                    </div>
                                  )}

                                  {/* History */}
                                  {detail.history.length > 0 && (
                                    <div class="history-section">
                                      <h4>Change History</h4>
                                      <ul class="history-list">
                                        {detail.history.map((h) => (
                                          <li key={h.id}>
                                            <span class="history-action">{h.action}</span>
                                            {' \u2014 '}
                                            {new Date(h.created_at).toLocaleString()}
                                          </li>
                                        ))}
                                      </ul>
                                    </div>
                                  )}
                                </>
                              )}
                            </div>
                          </td>
                        </tr>
                      )}
                    </>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          <div class="pagination">
            <button class="btn btn-sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>
              Previous
            </button>
            <span>Page {data.page} of {totalPages} ({data.total} total)</span>
            <button class="btn btn-sm" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>
              Next
            </button>
          </div>
        </>
      )}
    </div>
  );
}
