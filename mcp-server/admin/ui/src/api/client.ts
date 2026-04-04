export interface MCPRequest {
  id: number;
  tool_name: string;
  agent: string;
  params_summary: string;
  duration_ms: number;
  error: string;
  created_at: string;
}

export interface DashboardData {
  total_patterns: number;
  total_anti_patterns: number;
  recent_learning_count: number;
  owasp_counts: Record<string, number>;
  recent_scans: Array<{
    scan_type: string;
    last_run: string;
    findings_count: number;
  }>;
  session_info: {
    session_id: string;
    finding_count: number;
  } | null;
  trend_summary: Array<{
    scan_type: string;
    direction: string;
  }>;
}

export interface TrendsData {
  snapshots: Array<{
    scan_type: string;
    findings_count: number;
    created_at: string;
  }>;
  directions: Array<{
    scan_type: string;
    direction: string;
  }>;
}

export async function fetchActivity(limit = 100, offset = 0): Promise<MCPRequest[]> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  const res = await fetch(`/api/v1/activity?${params}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export async function fetchDashboard(): Promise<DashboardData> {
  const res = await fetch('/api/v1/dashboard');
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export async function fetchTrends(scanType?: string): Promise<TrendsData> {
  const params = new URLSearchParams();
  if (scanType) params.set('scan_type', scanType);
  const res = await fetch(`/api/v1/trends?${params}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

/* ── Pattern Management ── */

export interface PatternEntry {
  id: number;
  type: 'lint' | 'anti';
  rule?: string;
  file_glob?: string;
  pattern_id?: string;
  description?: string;
  category?: string;
  dont_code: string;
  do_code: string;
  frequency?: number;
  source: string;
  last_seen?: string;
  created_at: string;
  deleted_at: string | null;
}

export interface PatternsResponse {
  items: PatternEntry[];
  total: number;
  page: number;
  per_page: number;
}

export interface HistoryEntry {
  id: number;
  action: string;
  before_snapshot: string;
  after_snapshot: string;
  created_at: string;
}

export interface SuggestionEntry {
  type: string;
  pattern_ids: number[];
  description: string;
  action: string;
}

export async function fetchPatterns(params: {
  search?: string;
  source?: string;
  rule?: string;
  type?: string;
  sort?: string;
  include_deleted?: boolean;
  page?: number;
  per_page?: number;
}): Promise<PatternsResponse> {
  const q = new URLSearchParams();
  if (params.search) q.set('search', params.search);
  if (params.source) q.set('source', params.source);
  if (params.rule) q.set('rule', params.rule);
  if (params.type) q.set('type', params.type);
  if (params.sort) q.set('sort', params.sort);
  if (params.include_deleted) q.set('include_deleted', 'true');
  if (params.page) q.set('page', String(params.page));
  if (params.per_page) q.set('per_page', String(params.per_page));
  const res = await fetch(`/api/v1/patterns?${q}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export async function fetchPatternDetail(id: number, type: string): Promise<{ pattern: PatternEntry; history: HistoryEntry[] }> {
  const res = await fetch(`/api/v1/patterns/${id}?type=${type}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export async function updatePattern(id: number, data: { type: string; dont_code: string; do_code: string; rule?: string; file_glob?: string }): Promise<PatternEntry> {
  const res = await fetch(`/api/v1/patterns/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export async function deletePattern(id: number, type: string): Promise<void> {
  const res = await fetch(`/api/v1/patterns/${id}?type=${type}`, { method: 'DELETE' });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

export async function restorePattern(id: number, type: string): Promise<void> {
  const res = await fetch(`/api/v1/patterns/${id}/restore?type=${type}`, { method: 'POST' });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

export async function fetchSuggestions(): Promise<SuggestionEntry[]> {
  const res = await fetch('/api/v1/suggestions');
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

/* ── Session Findings ── */

export interface SessionFinding {
  id: number;
  agent: string;
  finding_type: string;
  file_path: string;
  description: string;
  severity: string;
  created_at: string;
}

export async function fetchSessionFindings(params?: {
  agent?: string;
  severity?: string;
  finding_type?: string;
  file_path?: string;
}): Promise<SessionFinding[]> {
  const q = new URLSearchParams();
  if (params?.agent) q.set('agent', params.agent);
  if (params?.severity) q.set('severity', params.severity);
  if (params?.finding_type) q.set('finding_type', params.finding_type);
  if (params?.file_path) q.set('file_path', params.file_path);
  const res = await fetch(`/api/v1/session-findings?${q}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

/* ── OWASP Findings ── */

export interface OWASPFinding {
  id: number;
  file_pattern: string;
  finding: string;
  fix_pattern: string;
  frequency: number;
}

export interface OWASPCategory {
  category: string;
  finding_count: number;
  findings: OWASPFinding[];
}

export interface OWASPResponse {
  categories: OWASPCategory[];
}

export async function fetchOWASP(category?: string): Promise<OWASPResponse> {
  const q = new URLSearchParams();
  if (category) q.set('category', category);
  const res = await fetch(`/api/v1/owasp?${q}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

/* ── Vulnerabilities ── */

export interface VulnDecision {
  decision: string;
  reason: string;
  cve_count: number;
  checked_at: string;
}

export interface Vulnerability {
  cve_id: string;
  severity: string;
  affected_versions: string;
  fixed_version: string;
  description: string;
  source: string;
  fetched_at: string;
}

export interface VulnModule {
  module: string;
  decision: VulnDecision | null;
  vulnerabilities: Vulnerability[];
}

export interface VulnResponse {
  modules: VulnModule[];
}

export async function fetchVulnerabilities(params?: {
  module?: string;
  severity?: string;
}): Promise<VulnResponse> {
  const q = new URLSearchParams();
  if (params?.module) q.set('module', params.module);
  if (params?.severity) q.set('severity', params.severity);
  const res = await fetch(`/api/v1/vulnerabilities?${q}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

/* ── Prefetch Status ── */

export interface PrefetchStatus {
  phase: string;
  source: string;
  progress: number;
  total: number;
  cves_found: number;
  cves_enriched: number;
  last_refresh: string;
  error: string;
}

export async function fetchPrefetchStatus(): Promise<PrefetchStatus> {
  const res = await fetch('/api/v1/prefetch-status');
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

/* ── Renovate ── */

export interface RenovatePreference {
  id: number;
  category: string;
  description: string;
  dont_config: string;
  do_config: string;
  frequency: number;
  source: string;
}

export interface RenovateRule {
  id: number;
  rule_id: string;
  category: string;
  title: string;
  description: string;
  dont_config: string;
  do_config: string;
  severity: string;
}

export interface RenovateConfigScore {
  id: number;
  config_path: string;
  score: number;
  findings_count: number;
  findings_detail: string;
  created_at: string;
}

export interface RenovateResponse {
  preferences: RenovatePreference[];
  rules: RenovateRule[];
  config_scores: RenovateConfigScore[];
}

export async function fetchRenovate(category?: string): Promise<RenovateResponse> {
  const q = new URLSearchParams();
  if (category) q.set('category', category);
  const res = await fetch(`/api/v1/renovate?${q}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}
