// Package db provides the persistent storage layer for go-guardian using SQLite.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed seed/*.sql
var seedFiles embed.FS

// LintPattern represents a lint rule pattern stored in the database.
type LintPattern struct {
	ID        int64
	Rule      string
	FileGlob  string
	DontCode  string
	DoCode    string
	Frequency int64
	Source    string
	LastSeen  time.Time
	CreatedAt time.Time
	DeletedAt *time.Time
}

// AntiPattern represents a known anti-pattern stored in the database.
type AntiPattern struct {
	ID          int64
	PatternID   string
	Description string
	DontCode    string
	DoCode      string
	Source      string
	Category    string
	CreatedAt   time.Time
	DeletedAt   *time.Time
}

// OWASPFinding represents a stored OWASP finding.
type OWASPFinding struct {
	ID          int64
	Category    string
	FilePattern string
	Finding     string
	FixPattern  string
	Frequency   int64
}

// VulnEntry represents a cached vulnerability entry.
type VulnEntry struct {
	Module           string
	CVEID            string
	Severity         string
	AffectedVersions string
	FixedVersion     string
	Description      string
	Source           string // "go-vuln" or "nvd"
	FetchedAt        time.Time
}

// DepDecision represents a recorded dependency decision.
type DepDecision struct {
	Module    string
	Decision  string
	Reason    string
	CVECount  int64
	CheckedAt time.Time
}

// ScanHistory represents a scan history record.
type ScanHistory struct {
	ScanType      string
	Project       string
	LastRun       time.Time
	FindingsCount int64
}

// ScanSnapshot represents an append-only snapshot of a scan result,
// used for trend tracking over time.
type ScanSnapshot struct {
	ID             int64
	ScanType       string
	Project        string
	FindingsCount  int64
	FindingsDetail string // JSON blob
	CreatedAt      time.Time
}

// RenovatePreference represents a learned Renovate config preference.
type RenovatePreference struct {
	ID          int64
	Category    string
	Description string
	DontConfig  string
	DoConfig    string
	Frequency   int64
	Source      string
	CreatedAt   time.Time
	LastSeen    time.Time
}

// RenovateRule represents a pre-seeded Renovate best-practice rule.
type RenovateRule struct {
	ID          int64
	RuleID      string
	Category    string
	Title       string
	Description string
	DontConfig  string
	DoConfig    string
	Severity    string
}

// ConfigScore represents a Renovate config score history entry.
type ConfigScore struct {
	ID             int64
	ConfigPath     string
	Score          int
	FindingsCount  int
	FindingsDetail string
	CreatedAt      time.Time
}

// PatternStats is an aggregate of pattern and scan statistics.
type PatternStats struct {
	TopLintPatterns   []LintPattern
	OWASPCounts       map[string]int64
	TotalLintPatterns int64
	TotalAntiPatterns int64
	RecentScans       []ScanHistory
}

// PatternHistory represents an audit log entry for pattern changes.
type PatternHistory struct {
	ID             int64
	PatternType    string // "lint" or "anti"
	PatternID      int64
	Action         string // "edit", "delete", "restore"
	BeforeSnapshot string // JSON
	AfterSnapshot  string // JSON
	CreatedAt      time.Time
}

// QualitySuggestion represents a server-side analysis suggestion for pattern quality.
type QualitySuggestion struct {
	Type        string  // "empty_do_code", "low_frequency", "duplicate_dont_code"
	PatternIDs  []int64
	Description string
	Action      string // "update", "merge", "remove"
}

// Store is the database access layer.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the SQLite database at dbPath, runs the schema
// migration, and seeds anti_patterns if the table is empty.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// SECURITY: restrict DB file to owner-only (fixes FINDING-06).
	// modernc.org/sqlite creates the file synchronously on Open.
	if err := os.Chmod(dbPath, 0o600); err != nil && !os.IsNotExist(err) {
		db.Close()
		return nil, fmt.Errorf("chmod guardian db: %w", err)
	}

	// Enable WAL mode and foreign keys for robustness.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragma setup: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := runSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema migration: %w", err)
	}

	s := &Store{db: db}

	// Seed anti_patterns if empty.
	var count int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM anti_patterns`).Scan(&count); err != nil {
		db.Close()
		return nil, fmt.Errorf("check anti_patterns count: %w", err)
	}
	if count == 0 {
		runSeedFiles(db)
	}

	// Seed renovate_rules if empty (separate check — renovate seeds are independent).
	var rcount int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM renovate_rules`).Scan(&rcount); err != nil {
		db.Close()
		return nil, fmt.Errorf("check renovate_rules count: %w", err)
	}
	if rcount == 0 {
		runSeedFiles(db)
	}

	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// HealthcheckTables returns all user table names in the database.
func (s *Store) HealthcheckTables() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// HealthcheckCounts returns row counts for all user tables.
func (s *Store) HealthcheckCounts() (map[string]int64, error) {
	tables, err := s.HealthcheckTables()
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(tables))
	for _, t := range tables {
		var count int64
		// Table name comes from sqlite_master, not user input — safe to interpolate.
		if err := s.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %q", t)).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s: %w", t, err)
		}
		counts[t] = count
	}
	return counts, nil
}

// runSchema executes the inlined DDL statements to create all tables and indexes.
func runSchema(db *sql.DB) error {
	if _, err := db.Exec(schemaStatements); err != nil {
		return err
	}
	// Soft-delete migration: add deleted_at column to existing tables.
	addColumnIfNotExists(db, "lint_patterns", "deleted_at", "DATETIME DEFAULT NULL")
	addColumnIfNotExists(db, "anti_patterns", "deleted_at", "DATETIME DEFAULT NULL")
	// Vuln source tracking migration.
	addColumnIfNotExists(db, "vuln_cache", "source", "TEXT NOT NULL DEFAULT 'go-vuln'")
	return nil
}

// addColumnIfNotExists attempts to add a column to a table, silently ignoring
// the error if the column already exists (duplicate column name).
func addColumnIfNotExists(db *sql.DB, table, column, colType string) {
	q := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colType)
	_, err := db.Exec(q)
	if err != nil && strings.Contains(err.Error(), "duplicate column name") {
		return // column already exists — safe to ignore
	}
	if err != nil {
		log.Printf("[go-guardian] warning: add column %s.%s: %v", table, column, err)
	}
}

// runSeedFiles reads and executes all *.sql files from the embedded seed
// directory. It is a no-op if the seed directory contains no .sql files.
// Each file is executed as a single Exec call so that multi-statement SQL with
// embedded semicolons inside string literals is handled correctly by the driver.
func runSeedFiles(db *sql.DB) {
	entries, err := fs.ReadDir(seedFiles, "seed")
	if err != nil {
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("[go-guardian] warning: seed transaction begin: %v", err)
		return
	}
	defer tx.Rollback()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := fs.ReadFile(seedFiles, "seed/"+entry.Name())
		if err != nil {
			continue
		}
		if _, err := tx.Exec(string(data)); err != nil {
			log.Printf("[go-guardian] warning: seed file %s execution error: %v", entry.Name(), err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[go-guardian] warning: seed transaction commit: %v", err)
	}
}

// InsertLintPattern upserts a lint pattern: on conflict it increments frequency
// and updates last_seen.
func (s *Store) InsertLintPattern(rule, fileGlob, dontCode, doCode, source string) error {
	const q = `
INSERT INTO lint_patterns (rule, file_glob, dont_code, do_code, source)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(rule, file_glob, dont_code) DO UPDATE SET
    frequency  = frequency + 1,
    last_seen  = CURRENT_TIMESTAMP,
    do_code    = excluded.do_code,
    source     = excluded.source,
    deleted_at = NULL`
	_, err := s.db.Exec(q, rule, fileGlob, dontCode, doCode, source)
	return err
}

// escapeLike escapes SQL LIKE wildcards in user input.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// QueryPatterns returns lint patterns whose file_glob matches the given glob
// string (substring LIKE match), ordered by frequency descending, up to limit
// rows. The codeContext parameter is reserved for future semantic filtering.
func (s *Store) QueryPatterns(fileGlob, codeContext string, limit int) ([]LintPattern, error) {
	const q = `
SELECT id, rule, file_glob, dont_code, do_code, frequency, source, last_seen
FROM lint_patterns
WHERE file_glob LIKE ? ESCAPE '\' AND deleted_at IS NULL
ORDER BY frequency DESC
LIMIT ?`
	rows, err := s.db.Query(q, "%"+escapeLike(fileGlob)+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []LintPattern
	for rows.Next() {
		var p LintPattern
		var lastSeen string
		if err := rows.Scan(&p.ID, &p.Rule, &p.FileGlob, &p.DontCode, &p.DoCode,
			&p.Frequency, &p.Source, &lastSeen); err != nil {
			return nil, err
		}
		p.LastSeen, _ = parseSQLiteTime(lastSeen)
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}

const owaspUpsertQ = `
INSERT INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (?, ?, ?, ?)
ON CONFLICT(category, file_pattern, finding) DO UPDATE SET
    frequency = frequency + 1,
    last_seen = CURRENT_TIMESTAMP,
    fix_pattern = CASE WHEN excluded.fix_pattern != '' THEN excluded.fix_pattern ELSE owasp_findings.fix_pattern END`

// InsertOWASPFinding upserts an OWASP finding: inserts on first encounter,
// increments frequency on subsequent calls for the same category/file_pattern/finding.
func (s *Store) InsertOWASPFinding(category, filePattern, finding, fixPattern string) error {
	_, err := s.db.Exec(owaspUpsertQ, category, filePattern, finding, fixPattern)
	return err
}

// OWASPFindingItem is a batch input item for InsertOWASPFindingsBatch.
type OWASPFindingItem struct {
	Category    string
	FilePattern string
	Finding     string
	FixPattern  string
}

// InsertOWASPFindingsBatch upserts multiple OWASP findings in a single transaction.
func (s *Store) InsertOWASPFindingsBatch(items []OWASPFindingItem) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(owaspUpsertQ)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range items {
		if _, err := stmt.Exec(item.Category, item.FilePattern, item.Finding, item.FixPattern); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// QueryOWASPFindings returns OWASP findings whose file_pattern matches the
// given glob string (substring LIKE match), ordered by frequency descending,
// up to limit rows.
func (s *Store) QueryOWASPFindings(fileGlob string, limit int) ([]OWASPFinding, error) {
	const q = `
SELECT id, category, file_pattern, finding, fix_pattern, frequency
FROM owasp_findings
WHERE file_pattern LIKE ? ESCAPE '\'
ORDER BY frequency DESC
LIMIT ?`
	rows, err := s.db.Query(q, "%"+escapeLike(fileGlob)+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var findings []OWASPFinding
	for rows.Next() {
		var f OWASPFinding
		if err := rows.Scan(&f.ID, &f.Category, &f.FilePattern, &f.Finding, &f.FixPattern, &f.Frequency); err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	return findings, rows.Err()
}

// UpdateScanHistory upserts a scan history record, refreshing last_run and
// findings_count on conflict.
func (s *Store) UpdateScanHistory(scanType, project string, findingsCount int) error {
	const q = `
INSERT INTO scan_history (scan_type, project, findings_count)
VALUES (?, ?, ?)
ON CONFLICT(scan_type, project) DO UPDATE SET
    last_run       = CURRENT_TIMESTAMP,
    findings_count = excluded.findings_count`
	_, err := s.db.Exec(q, scanType, project, findingsCount)
	return err
}

// InsertAntiPattern inserts an anti-pattern using INSERT OR IGNORE so that
// duplicate pattern_id entries are silently skipped.
func (s *Store) InsertAntiPattern(patternID, description, dontCode, doCode, source, category string) error {
	const q = `
INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(q, patternID, description, dontCode, doCode, source, category)
	return err
}

// QueryAntiPatterns returns all anti-patterns for the given category.
// If category is empty, all anti-patterns are returned.
func (s *Store) QueryAntiPatterns(category string) ([]AntiPattern, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if category == "" {
		rows, err = s.db.Query(
			`SELECT pattern_id, description, dont_code, do_code, source, category FROM anti_patterns WHERE deleted_at IS NULL`,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT pattern_id, description, dont_code, do_code, source, category FROM anti_patterns WHERE category=? AND deleted_at IS NULL`,
			category,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aps []AntiPattern
	for rows.Next() {
		var ap AntiPattern
		if err := rows.Scan(&ap.PatternID, &ap.Description, &ap.DontCode, &ap.DoCode,
			&ap.Source, &ap.Category); err != nil {
			return nil, err
		}
		aps = append(aps, ap)
	}
	return aps, rows.Err()
}

// UpsertVulnCache inserts or updates a vulnerability cache entry.
func (s *Store) UpsertVulnCache(module, cveID, severity, affected, fixed, description, source string) error {
	const q = `
INSERT INTO vuln_cache (module, cve_id, severity, affected_versions, fixed_version, description, source)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(module, cve_id) DO UPDATE SET
    severity          = excluded.severity,
    affected_versions = excluded.affected_versions,
    fixed_version     = excluded.fixed_version,
    description       = excluded.description,
    source            = excluded.source,
    fetched_at        = CURRENT_TIMESTAMP`
	_, err := s.db.Exec(q, module, cveID, severity, affected, fixed, description, source)
	return err
}

// GetVulnCache retrieves all cached vulnerabilities for the given module.
func (s *Store) GetVulnCache(module string) ([]VulnEntry, error) {
	const q = `
SELECT module, cve_id, severity, affected_versions, fixed_version, description, source, fetched_at
FROM vuln_cache
WHERE module=?`
	rows, err := s.db.Query(q, module)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []VulnEntry
	for rows.Next() {
		var e VulnEntry
		var fetchedAt string
		if err := rows.Scan(&e.Module, &e.CVEID, &e.Severity, &e.AffectedVersions,
			&e.FixedVersion, &e.Description, &e.Source, &fetchedAt); err != nil {
			return nil, err
		}
		e.FetchedAt, _ = parseSQLiteTime(fetchedAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// UpsertDepDecision inserts or updates a dependency decision.
func (s *Store) UpsertDepDecision(module, decision, reason string, cveCount int) error {
	const q = `
INSERT INTO dep_decisions (module, decision, reason, cve_count)
VALUES (?, ?, ?, ?)
ON CONFLICT(module) DO UPDATE SET
    decision   = excluded.decision,
    reason     = excluded.reason,
    cve_count  = excluded.cve_count,
    checked_at = CURRENT_TIMESTAMP`
	_, err := s.db.Exec(q, module, decision, reason, cveCount)
	return err
}

// GetDepDecision retrieves the dependency decision for the given module.
// Returns nil, nil if no decision has been recorded.
func (s *Store) GetDepDecision(module string) (*DepDecision, error) {
	const q = `SELECT module, decision, reason, cve_count, checked_at FROM dep_decisions WHERE module=?`
	var d DepDecision
	var checkedAt string
	err := s.db.QueryRow(q, module).Scan(&d.Module, &d.Decision, &d.Reason, &d.CVECount, &checkedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.CheckedAt, _ = parseSQLiteTime(checkedAt)
	return &d, nil
}

// GetAllVulnEntries returns all vulnerability cache entries, ordered by
// severity descending then module ascending. Default limit is 500 if <= 0.
func (s *Store) GetAllVulnEntries(limit int) ([]VulnEntry, error) {
	if limit <= 0 {
		limit = 500
	}
	const q = `
SELECT module, cve_id, severity, affected_versions, fixed_version, description, source, fetched_at
FROM vuln_cache
ORDER BY severity DESC, module ASC
LIMIT ?`
	rows, err := s.db.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []VulnEntry
	for rows.Next() {
		var e VulnEntry
		var fetchedAt string
		if err := rows.Scan(&e.Module, &e.CVEID, &e.Severity, &e.AffectedVersions,
			&e.FixedVersion, &e.Description, &e.Source, &fetchedAt); err != nil {
			return nil, err
		}
		e.FetchedAt, _ = parseSQLiteTime(fetchedAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetAllDepDecisions returns all dependency decisions, ordered by checked_at descending.
func (s *Store) GetAllDepDecisions() ([]DepDecision, error) {
	const q = `
SELECT module, decision, reason, cve_count, checked_at
FROM dep_decisions
ORDER BY checked_at DESC`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decisions []DepDecision
	for rows.Next() {
		var d DepDecision
		var checkedAt string
		if err := rows.Scan(&d.Module, &d.Decision, &d.Reason, &d.CVECount, &checkedAt); err != nil {
			return nil, err
		}
		d.CheckedAt, _ = parseSQLiteTime(checkedAt)
		decisions = append(decisions, d)
	}
	return decisions, rows.Err()
}

// GetScanHistory retrieves all scan history records for the given project,
// ordered most-recent first.
func (s *Store) GetScanHistory(project string) ([]ScanHistory, error) {
	const q = `
SELECT scan_type, project, last_run, findings_count
FROM scan_history
WHERE project=?
ORDER BY last_run DESC`
	rows, err := s.db.Query(q, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []ScanHistory
	for rows.Next() {
		var h ScanHistory
		var lastRun string
		if err := rows.Scan(&h.ScanType, &h.Project, &lastRun, &h.FindingsCount); err != nil {
			return nil, err
		}
		h.LastRun, _ = parseSQLiteTime(lastRun)
		history = append(history, h)
	}
	return history, rows.Err()
}

// GetPatternStats returns aggregate statistics: top 10 lint patterns by
// frequency, OWASP finding counts by category, total counts for lint/anti
// patterns, and recent scans for the given project.
func (s *Store) GetPatternStats(project string) (*PatternStats, error) {
	stats := &PatternStats{
		OWASPCounts: make(map[string]int64),
	}

	// Top 10 lint patterns by frequency.
	const topQ = `
SELECT id, rule, file_glob, dont_code, do_code, frequency, source, last_seen
FROM lint_patterns
WHERE deleted_at IS NULL
ORDER BY frequency DESC
LIMIT 10`
	topRows, err := s.db.Query(topQ)
	if err != nil {
		return nil, fmt.Errorf("top lint patterns: %w", err)
	}
	defer topRows.Close()
	for topRows.Next() {
		var p LintPattern
		var lastSeen string
		if err := topRows.Scan(&p.ID, &p.Rule, &p.FileGlob, &p.DontCode, &p.DoCode,
			&p.Frequency, &p.Source, &lastSeen); err != nil {
			return nil, err
		}
		p.LastSeen, _ = parseSQLiteTime(lastSeen)
		stats.TopLintPatterns = append(stats.TopLintPatterns, p)
	}
	if err := topRows.Err(); err != nil {
		return nil, err
	}

	// OWASP counts by category.
	const owaspQ = `SELECT category, COUNT(*) FROM owasp_findings GROUP BY category`
	owaspRows, err := s.db.Query(owaspQ)
	if err != nil {
		return nil, fmt.Errorf("owasp counts: %w", err)
	}
	defer owaspRows.Close()
	for owaspRows.Next() {
		var cat string
		var cnt int64
		if err := owaspRows.Scan(&cat, &cnt); err != nil {
			return nil, err
		}
		stats.OWASPCounts[cat] = cnt
	}
	if err := owaspRows.Err(); err != nil {
		return nil, err
	}

	// Total counts for lint and anti-patterns in a single query.
	const countQ = `SELECT 'lint' AS type, COUNT(*) AS cnt FROM lint_patterns WHERE deleted_at IS NULL
UNION ALL
SELECT 'anti', COUNT(*) FROM anti_patterns WHERE deleted_at IS NULL`
	countRows, err := s.db.Query(countQ)
	if err != nil {
		return nil, fmt.Errorf("total counts: %w", err)
	}
	defer countRows.Close()
	for countRows.Next() {
		var typ string
		var cnt int64
		if err := countRows.Scan(&typ, &cnt); err != nil {
			return nil, err
		}
		switch typ {
		case "lint":
			stats.TotalLintPatterns = cnt
		case "anti":
			stats.TotalAntiPatterns = cnt
		}
	}
	if err := countRows.Err(); err != nil {
		return nil, err
	}

	// Recent scans for the project (latest 10).
	const scanQ = `
SELECT scan_type, project, last_run, findings_count
FROM scan_history
WHERE project=?
ORDER BY last_run DESC
LIMIT 10`
	scanRows, err := s.db.Query(scanQ, project)
	if err != nil {
		return nil, fmt.Errorf("recent scans: %w", err)
	}
	defer scanRows.Close()
	for scanRows.Next() {
		var h ScanHistory
		var lastRun string
		if err := scanRows.Scan(&h.ScanType, &h.Project, &lastRun, &h.FindingsCount); err != nil {
			return nil, err
		}
		h.LastRun, _ = parseSQLiteTime(lastRun)
		stats.RecentScans = append(stats.RecentScans, h)
	}
	if err := scanRows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

// RecentLearningCount returns the number of lint patterns created in the last N days.
func (s *Store) RecentLearningCount(days int) (int64, error) {
	var count int64
	q := fmt.Sprintf(`SELECT COUNT(*) FROM lint_patterns WHERE created_at >= datetime('now', '-%d days') AND deleted_at IS NULL`, days)
	err := s.db.QueryRow(q).Scan(&count)
	return count, err
}

// ── Scan Snapshots (trend tracking) ──────────────────────────────────────────

const snapshotRetention = 100

// InsertScanSnapshot appends a scan snapshot and prunes old entries beyond
// the retention limit for the same (scan_type, project) pair.
func (s *Store) InsertScanSnapshot(scanType, project string, findingsCount int, findingsDetail string) error {
	if findingsDetail == "" {
		findingsDetail = "{}"
	}
	const insertQ = `
INSERT INTO scan_snapshots (scan_type, project, findings_count, findings_detail)
VALUES (?, ?, ?, ?)`
	if _, err := s.db.Exec(insertQ, scanType, project, findingsCount, findingsDetail); err != nil {
		return err
	}

	// Prune oldest entries beyond retention limit using monotonic id boundary.
	const pruneQ = `
DELETE FROM scan_snapshots
WHERE scan_type=? AND project=?
AND id <= (
    SELECT id FROM scan_snapshots
    WHERE scan_type=? AND project=?
    ORDER BY id DESC
    LIMIT 1 OFFSET ?
)`
	_, err := s.db.Exec(pruneQ, scanType, project, scanType, project, snapshotRetention)
	return err
}

// GetScanSnapshots retrieves the last N snapshots for a (scan_type, project)
// pair, ordered most-recent first.
func (s *Store) GetScanSnapshots(scanType, project string, limit int) ([]ScanSnapshot, error) {
	const q = `
SELECT id, scan_type, project, findings_count, findings_detail, created_at
FROM scan_snapshots
WHERE scan_type=? AND project=?
ORDER BY id DESC
LIMIT ?`
	rows, err := s.db.Query(q, scanType, project, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSnapshotsFromRows(rows)
}

// GetAllScanSnapshots retrieves the last N snapshots for all scan types for a
// project, ordered most-recent first.
func (s *Store) GetAllScanSnapshots(project string, limit int) ([]ScanSnapshot, error) {
	const q = `
SELECT id, scan_type, project, findings_count, findings_detail, created_at
FROM scan_snapshots
WHERE project=?
ORDER BY id DESC
LIMIT ?`
	rows, err := s.db.Query(q, project, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSnapshotsFromRows(rows)
}

func scanSnapshotsFromRows(rows *sql.Rows) ([]ScanSnapshot, error) {
	var snapshots []ScanSnapshot
	for rows.Next() {
		var ss ScanSnapshot
		var createdAt string
		if err := rows.Scan(&ss.ID, &ss.ScanType, &ss.Project, &ss.FindingsCount,
			&ss.FindingsDetail, &createdAt); err != nil {
			return nil, err
		}
		ss.CreatedAt, _ = parseSQLiteTime(createdAt)
		snapshots = append(snapshots, ss)
	}
	return snapshots, rows.Err()
}

// ── Session Findings (cross-agent buffer) ────────────────────────────────────

// SessionFinding represents a finding reported by an agent during a session.
type SessionFinding struct {
	ID          int64
	SessionID   string
	Agent       string
	FindingType string
	FilePath    string
	Description string
	Severity    string
	CreatedAt   time.Time
}

// InsertSessionFinding records a finding for the current session.
func (s *Store) InsertSessionFinding(sessionID, agent, findingType, filePath, description, severity string) (int64, error) {
	const q = `
INSERT INTO session_findings (session_id, agent, finding_type, file_path, description, severity)
VALUES (?, ?, ?, ?, ?, ?)`
	result, err := s.db.Exec(q, sessionID, agent, findingType, filePath, description, severity)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetSessionFindings returns all findings for the given session, optionally
// filtered by agent. If agent is empty, all findings for the session are returned.
func (s *Store) GetSessionFindings(sessionID, agent string) ([]SessionFinding, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if agent == "" {
		rows, err = s.db.Query(
			`SELECT id, session_id, agent, finding_type, file_path, description, severity, created_at
			FROM session_findings WHERE session_id=? ORDER BY created_at`,
			sessionID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, session_id, agent, finding_type, file_path, description, severity, created_at
			FROM session_findings WHERE session_id=? AND agent=? ORDER BY created_at`,
			sessionID, agent,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return sessionFindingsFromRows(rows)
}

// GetSessionFindingsByFile returns all findings for the given session that
// match the file path (basename match via LIKE).
func (s *Store) GetSessionFindingsByFile(sessionID, filePath string) ([]SessionFinding, error) {
	const q = `
SELECT id, session_id, agent, finding_type, file_path, description, severity, created_at
FROM session_findings
WHERE session_id=? AND file_path LIKE ? ESCAPE '\'
ORDER BY created_at`
	rows, err := s.db.Query(q, sessionID, "%"+escapeLike(filePath)+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return sessionFindingsFromRows(rows)
}

// CleanupOldSessions deletes all session_findings rows not matching the
// current session_id.
func (s *Store) CleanupOldSessions(currentSessionID string) error {
	_, err := s.db.Exec(`DELETE FROM session_findings WHERE session_id != ?`, currentSessionID)
	return err
}

func sessionFindingsFromRows(rows *sql.Rows) ([]SessionFinding, error) {
	var findings []SessionFinding
	for rows.Next() {
		var f SessionFinding
		var createdAt string
		if err := rows.Scan(&f.ID, &f.SessionID, &f.Agent, &f.FindingType,
			&f.FilePath, &f.Description, &f.Severity, &createdAt); err != nil {
			return nil, err
		}
		f.CreatedAt, _ = parseSQLiteTime(createdAt)
		findings = append(findings, f)
	}
	return findings, rows.Err()
}

// MCPRequest represents a logged MCP tool invocation for the admin activity log.
type MCPRequest struct {
	ID            int64
	ToolName      string
	Agent         string
	ParamsSummary string
	DurationMS    int64
	Error         string
	CreatedAt     time.Time
}

// ── MCP Request Logging (admin activity log) ────────────────────────────────

// InsertMCPRequest logs an MCP tool invocation.
func (s *Store) InsertMCPRequest(toolName, agent, paramsSummary string, durationMS int64, errMsg string) error {
	const q = `
INSERT INTO mcp_requests (tool_name, agent, params_summary, duration_ms, error)
VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.Exec(q, toolName, agent, paramsSummary, durationMS, errMsg)
	return err
}

// GetMCPRequests returns recent MCP tool invocations, optionally filtered by
// tool_name and/or agent. Results are ordered most-recent first.
func (s *Store) GetMCPRequests(toolName, agent string, limit, offset int) ([]MCPRequest, error) {
	if limit <= 0 {
		limit = 100
	}
	var conditions []string
	var args []any
	if toolName != "" {
		conditions = append(conditions, "tool_name = ?")
		args = append(args, toolName)
	}
	if agent != "" {
		conditions = append(conditions, "agent = ?")
		args = append(args, agent)
	}
	q := "SELECT id, tool_name, agent, params_summary, duration_ms, error, created_at FROM mcp_requests"
	if len(conditions) > 0 {
		q += " WHERE " + strings.Join(conditions, " AND ")
	}
	q += " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []MCPRequest
	for rows.Next() {
		var r MCPRequest
		var createdAt string
		if err := rows.Scan(&r.ID, &r.ToolName, &r.Agent, &r.ParamsSummary,
			&r.DurationMS, &r.Error, &createdAt); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = parseSQLiteTime(createdAt)
		requests = append(requests, r)
	}
	return requests, rows.Err()
}

// PruneMCPRequests deletes mcp_requests entries older than the given duration.
// Returns the number of rows deleted.
func (s *Store) PruneMCPRequests(olderThan time.Duration) (int64, error) {
	const q = `DELETE FROM mcp_requests WHERE created_at < datetime('now', ?)`
	threshold := fmt.Sprintf("-%d seconds", int(olderThan.Seconds()))
	result, err := s.db.Exec(q, threshold)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// parseSQLiteTime parses the datetime formats that SQLite's CURRENT_TIMESTAMP
// may produce. Returns the zero time on parse failure (error is discarded by
// callers via the blank identifier).
func parseSQLiteTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05", // SQLite CURRENT_TIMESTAMP default — most common
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable sqlite time: %q", s)
}

// schemaStatements is the DDL for all six tables and their indexes.
// It mirrors schema.sql exactly and is used by runSchema so that the store
// package has no external file dependency at runtime.
const schemaStatements = `
CREATE TABLE IF NOT EXISTS lint_patterns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    rule TEXT NOT NULL,
    file_glob TEXT NOT NULL DEFAULT '*',
    dont_code TEXT NOT NULL,
    do_code TEXT NOT NULL,
    frequency INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'learned',
    last_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME DEFAULT NULL,
    UNIQUE(rule, file_glob, dont_code)
);

CREATE TABLE IF NOT EXISTS owasp_findings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT NOT NULL,
    file_pattern TEXT NOT NULL DEFAULT '*',
    finding TEXT NOT NULL,
    fix_pattern TEXT NOT NULL DEFAULT '',
    frequency INTEGER NOT NULL DEFAULT 1,
    last_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS vuln_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module TEXT NOT NULL,
    cve_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    affected_versions TEXT NOT NULL,
    fixed_version TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'go-vuln',
    fetched_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(module, cve_id)
);

CREATE TABLE IF NOT EXISTS scan_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_type TEXT NOT NULL,
    project TEXT NOT NULL,
    last_run DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    findings_count INTEGER NOT NULL DEFAULT 0,
    UNIQUE(scan_type, project)
);

CREATE TABLE IF NOT EXISTS anti_patterns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern_id TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    dont_code TEXT NOT NULL,
    do_code TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'notque',
    category TEXT NOT NULL DEFAULT 'general',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS dep_decisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module TEXT NOT NULL UNIQUE,
    decision TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    cve_count INTEGER NOT NULL DEFAULT 0,
    checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_lint_patterns_rule ON lint_patterns(rule);
CREATE INDEX IF NOT EXISTS idx_lint_patterns_frequency ON lint_patterns(frequency DESC);
CREATE INDEX IF NOT EXISTS idx_owasp_findings_category ON owasp_findings(category);
CREATE UNIQUE INDEX IF NOT EXISTS idx_owasp_findings_unique ON owasp_findings(category, file_pattern, finding);
CREATE INDEX IF NOT EXISTS idx_vuln_cache_module ON vuln_cache(module);
CREATE INDEX IF NOT EXISTS idx_scan_history_project ON scan_history(project);
CREATE INDEX IF NOT EXISTS idx_anti_patterns_category ON anti_patterns(category);

CREATE TABLE IF NOT EXISTS scan_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_type TEXT NOT NULL,
    project TEXT NOT NULL,
    findings_count INTEGER NOT NULL DEFAULT 0,
    findings_detail TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scan_snapshots_lookup
    ON scan_snapshots(project, scan_type, created_at DESC);

CREATE TABLE IF NOT EXISTS session_findings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    agent TEXT NOT NULL,
    finding_type TEXT NOT NULL,
    file_path TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'MEDIUM',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_session_findings_session
    ON session_findings(session_id);
CREATE INDEX IF NOT EXISTS idx_session_findings_lookup
    ON session_findings(session_id, agent);

CREATE TABLE IF NOT EXISTS renovate_preferences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT NOT NULL,
    description TEXT NOT NULL,
    dont_config TEXT NOT NULL DEFAULT '',
    do_config TEXT NOT NULL DEFAULT '',
    frequency INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'learned',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_renovate_preferences_category
    ON renovate_preferences(category);

CREATE TABLE IF NOT EXISTS renovate_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id TEXT NOT NULL UNIQUE,
    category TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    dont_config TEXT NOT NULL DEFAULT '',
    do_config TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL DEFAULT 'INFO'
);
CREATE INDEX IF NOT EXISTS idx_renovate_rules_category
    ON renovate_rules(category);

CREATE TABLE IF NOT EXISTS config_scores (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    config_path TEXT NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    findings_count INTEGER NOT NULL DEFAULT 0,
    findings_detail TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_config_scores_path
    ON config_scores(config_path, created_at DESC);

CREATE TABLE IF NOT EXISTS mcp_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tool_name TEXT NOT NULL,
    agent TEXT NOT NULL DEFAULT '',
    params_summary TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_mcp_requests_created
    ON mcp_requests(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mcp_requests_tool
    ON mcp_requests(tool_name);

CREATE TABLE IF NOT EXISTS pattern_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern_type TEXT NOT NULL,
    pattern_id INTEGER NOT NULL,
    action TEXT NOT NULL,
    before_snapshot TEXT NOT NULL DEFAULT '{}',
    after_snapshot TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_pattern_history_lookup
    ON pattern_history(pattern_type, pattern_id, created_at DESC);
`

// --- Renovate Preferences ---

// InsertRenovatePreference inserts a new preference or increments frequency if a matching one exists.
func (s *Store) InsertRenovatePreference(category, description, dontConfig, doConfig string) error {
	var existingID int64
	err := s.db.QueryRow(
		`SELECT id FROM renovate_preferences WHERE category = ? AND description = ? LIMIT 1`,
		category, description,
	).Scan(&existingID)

	if err == nil {
		_, err = s.db.Exec(
			`UPDATE renovate_preferences SET frequency = frequency + 1, last_seen = CURRENT_TIMESTAMP WHERE id = ?`,
			existingID,
		)
		return err
	}

	_, err = s.db.Exec(
		`INSERT INTO renovate_preferences (category, description, dont_config, do_config) VALUES (?, ?, ?, ?)`,
		category, description, dontConfig, doConfig,
	)
	return err
}

// QueryRenovatePreferences returns preferences matching the given category.
func (s *Store) QueryRenovatePreferences(category string, limit int) ([]RenovatePreference, error) {
	var rows *sql.Rows
	var err error

	if category != "" {
		rows, err = s.db.Query(
			`SELECT id, category, description, dont_config, do_config, frequency, source, created_at, last_seen
			 FROM renovate_preferences WHERE category = ? ORDER BY frequency DESC LIMIT ?`,
			category, limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, category, description, dont_config, do_config, frequency, source, created_at, last_seen
			 FROM renovate_preferences ORDER BY frequency DESC LIMIT ?`,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prefs []RenovatePreference
	for rows.Next() {
		var p RenovatePreference
		if err := rows.Scan(&p.ID, &p.Category, &p.Description, &p.DontConfig, &p.DoConfig,
			&p.Frequency, &p.Source, &p.CreatedAt, &p.LastSeen); err != nil {
			return nil, err
		}
		prefs = append(prefs, p)
	}
	return prefs, rows.Err()
}

// --- Renovate Rules ---

// QueryRenovateRules returns rules matching the given category.
func (s *Store) QueryRenovateRules(category string) ([]RenovateRule, error) {
	var rows *sql.Rows
	var err error

	if category != "" {
		rows, err = s.db.Query(
			`SELECT id, rule_id, category, title, description, dont_config, do_config, severity
			 FROM renovate_rules WHERE category = ?
			 ORDER BY CASE severity WHEN 'CRITICAL' THEN 1 WHEN 'WARN' THEN 2 ELSE 3 END`,
			category,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, rule_id, category, title, description, dont_config, do_config, severity
			 FROM renovate_rules
			 ORDER BY CASE severity WHEN 'CRITICAL' THEN 1 WHEN 'WARN' THEN 2 ELSE 3 END`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []RenovateRule
	for rows.Next() {
		var r RenovateRule
		if err := rows.Scan(&r.ID, &r.RuleID, &r.Category, &r.Title, &r.Description,
			&r.DontConfig, &r.DoConfig, &r.Severity); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// SearchRenovateRules returns rules whose title or description match the query.
func (s *Store) SearchRenovateRules(query string, limit int) ([]RenovateRule, error) {
	pattern := "%" + escapeLike(query) + "%"
	rows, err := s.db.Query(
		`SELECT id, rule_id, category, title, description, dont_config, do_config, severity
		 FROM renovate_rules WHERE title LIKE ? ESCAPE '\' OR description LIKE ? ESCAPE '\'
		 ORDER BY CASE severity WHEN 'CRITICAL' THEN 1 WHEN 'WARN' THEN 2 ELSE 3 END
		 LIMIT ?`,
		pattern, pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []RenovateRule
	for rows.Next() {
		var r RenovateRule
		if err := rows.Scan(&r.ID, &r.RuleID, &r.Category, &r.Title, &r.Description,
			&r.DontConfig, &r.DoConfig, &r.Severity); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// --- Config Scores ---

// InsertConfigScore records a new config score entry.
func (s *Store) InsertConfigScore(configPath string, score, findingsCount int, findingsDetail string) error {
	_, err := s.db.Exec(
		`INSERT INTO config_scores (config_path, score, findings_count, findings_detail) VALUES (?, ?, ?, ?)`,
		configPath, score, findingsCount, findingsDetail,
	)
	return err
}

// GetConfigScores returns the most recent config scores for a given path.
func (s *Store) GetConfigScores(configPath string, limit int) ([]ConfigScore, error) {
	rows, err := s.db.Query(
		`SELECT id, config_path, score, findings_count, findings_detail, created_at
		 FROM config_scores WHERE config_path = ? ORDER BY created_at DESC, id DESC LIMIT ?`,
		configPath, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scores []ConfigScore
	for rows.Next() {
		var cs ConfigScore
		if err := rows.Scan(&cs.ID, &cs.ConfigPath, &cs.Score, &cs.FindingsCount,
			&cs.FindingsDetail, &cs.CreatedAt); err != nil {
			return nil, err
		}
		scores = append(scores, cs)
	}
	return scores, rows.Err()
}

// GetRecentConfigScores returns the most recent scores across all paths.
func (s *Store) GetRecentConfigScores(limit int) ([]ConfigScore, error) {
	rows, err := s.db.Query(
		`SELECT id, config_path, score, findings_count, findings_detail, created_at
		 FROM config_scores ORDER BY created_at DESC, id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scores []ConfigScore
	for rows.Next() {
		var cs ConfigScore
		if err := rows.Scan(&cs.ID, &cs.ConfigPath, &cs.Score, &cs.FindingsCount,
			&cs.FindingsDetail, &cs.CreatedAt); err != nil {
			return nil, err
		}
		scores = append(scores, cs)
	}
	return scores, rows.Err()
}

// --- Renovate Stats ---

// RenovateRuleCountByCategory returns the count of renovate rules per category.
func (s *Store) RenovateRuleCountByCategory() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT category, COUNT(*) FROM renovate_rules GROUP BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var cat string
		var count int
		if err := rows.Scan(&cat, &count); err != nil {
			return nil, err
		}
		counts[cat] = count
	}
	return counts, rows.Err()
}

// RenovatePreferenceCount returns the total number of learned renovate preferences.
func (s *Store) RenovatePreferenceCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM renovate_preferences`).Scan(&count)
	return count, err
}

// TotalRenovateRuleCount returns the total number of renovate rules.
func (s *Store) TotalRenovateRuleCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM renovate_rules`).Scan(&count)
	return count, err
}

// ── Pattern Management (admin UI) ──────────────────────────────────────────

// InsertPatternHistory records an audit log entry for a pattern change.
func (s *Store) InsertPatternHistory(patternType string, patternID int64, action, beforeSnapshot, afterSnapshot string) error {
	const q = `
INSERT INTO pattern_history (pattern_type, pattern_id, action, before_snapshot, after_snapshot)
VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.Exec(q, patternType, patternID, action, beforeSnapshot, afterSnapshot)
	return err
}

// GetPatternHistory returns audit log entries for a specific pattern,
// ordered most-recent first.
func (s *Store) GetPatternHistory(patternType string, patternID int64) ([]PatternHistory, error) {
	const q = `
SELECT id, pattern_type, pattern_id, action, before_snapshot, after_snapshot, created_at
FROM pattern_history
WHERE pattern_type=? AND pattern_id=?
ORDER BY created_at DESC, id DESC`
	rows, err := s.db.Query(q, patternType, patternID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return patternHistoryFromRows(rows)
}

// GetRecentPatternHistory returns the most recent audit log entries across
// all patterns, ordered most-recent first.
func (s *Store) GetRecentPatternHistory(limit int) ([]PatternHistory, error) {
	const q = `
SELECT id, pattern_type, pattern_id, action, before_snapshot, after_snapshot, created_at
FROM pattern_history
ORDER BY created_at DESC, id DESC
LIMIT ?`
	rows, err := s.db.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return patternHistoryFromRows(rows)
}

func patternHistoryFromRows(rows *sql.Rows) ([]PatternHistory, error) {
	var entries []PatternHistory
	for rows.Next() {
		var h PatternHistory
		var createdAt string
		if err := rows.Scan(&h.ID, &h.PatternType, &h.PatternID, &h.Action,
			&h.BeforeSnapshot, &h.AfterSnapshot, &createdAt); err != nil {
			return nil, err
		}
		h.CreatedAt, _ = parseSQLiteTime(createdAt)
		entries = append(entries, h)
	}
	return entries, rows.Err()
}

// GetAllLintPatterns returns lint patterns for the admin browser with search,
// filter, sort, pagination, and optional soft-deleted pattern inclusion.
func (s *Store) GetAllLintPatterns(search, source, rule, sortBy string, includeDeleted bool, limit, offset int) ([]LintPattern, int64, error) {
	var conditions []string
	var args []any

	if !includeDeleted {
		conditions = append(conditions, "deleted_at IS NULL")
	}
	if search != "" {
		pattern := "%" + escapeLike(search) + "%"
		conditions = append(conditions, "(rule LIKE ? ESCAPE '\\' OR dont_code LIKE ? ESCAPE '\\' OR do_code LIKE ? ESCAPE '\\' OR file_glob LIKE ? ESCAPE '\\')")
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if source != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, source)
	}
	if rule != "" {
		conditions = append(conditions, "rule = ?")
		args = append(args, rule)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching rows.
	var total int64
	countQ := "SELECT COUNT(*) FROM lint_patterns" + whereClause
	if err := s.db.QueryRow(countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count lint patterns: %w", err)
	}

	// Determine sort order.
	orderBy := "frequency DESC"
	switch sortBy {
	case "last_seen":
		orderBy = "last_seen DESC"
	case "created_at":
		orderBy = "created_at DESC"
	}

	q := "SELECT id, rule, file_glob, dont_code, do_code, frequency, source, last_seen, created_at, deleted_at FROM lint_patterns" +
		whereClause + " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
	queryArgs := append(args, limit, offset)

	rows, err := s.db.Query(q, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var patterns []LintPattern
	for rows.Next() {
		var p LintPattern
		var lastSeen, createdAt string
		var deletedAt sql.NullString
		if err := rows.Scan(&p.ID, &p.Rule, &p.FileGlob, &p.DontCode, &p.DoCode,
			&p.Frequency, &p.Source, &lastSeen, &createdAt, &deletedAt); err != nil {
			return nil, 0, err
		}
		p.LastSeen, _ = parseSQLiteTime(lastSeen)
		p.CreatedAt, _ = parseSQLiteTime(createdAt)
		if deletedAt.Valid {
			t, _ := parseSQLiteTime(deletedAt.String)
			p.DeletedAt = &t
		}
		patterns = append(patterns, p)
	}
	return patterns, total, rows.Err()
}

// GetLintPatternByID returns a single lint pattern by ID (including soft-deleted ones).
func (s *Store) GetLintPatternByID(id int64) (*LintPattern, error) {
	const q = `
SELECT id, rule, file_glob, dont_code, do_code, frequency, source, last_seen, created_at, deleted_at
FROM lint_patterns WHERE id=?`
	var p LintPattern
	var lastSeen, createdAt string
	var deletedAt sql.NullString
	err := s.db.QueryRow(q, id).Scan(&p.ID, &p.Rule, &p.FileGlob, &p.DontCode, &p.DoCode,
		&p.Frequency, &p.Source, &lastSeen, &createdAt, &deletedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.LastSeen, _ = parseSQLiteTime(lastSeen)
	p.CreatedAt, _ = parseSQLiteTime(createdAt)
	if deletedAt.Valid {
		t, _ := parseSQLiteTime(deletedAt.String)
		p.DeletedAt = &t
	}
	return &p, nil
}

// UpdateLintPattern updates the mutable fields of a lint pattern.
func (s *Store) UpdateLintPattern(id int64, dontCode, doCode, rule, fileGlob string) error {
	const q = `UPDATE lint_patterns SET dont_code=?, do_code=?, rule=?, file_glob=? WHERE id=?`
	_, err := s.db.Exec(q, dontCode, doCode, rule, fileGlob, id)
	return err
}

// SoftDeleteLintPattern marks a lint pattern as deleted.
func (s *Store) SoftDeleteLintPattern(id int64) error {
	const q = `UPDATE lint_patterns SET deleted_at = CURRENT_TIMESTAMP WHERE id=? AND deleted_at IS NULL`
	_, err := s.db.Exec(q, id)
	return err
}

// RestoreLintPattern clears the deleted_at flag on a soft-deleted lint pattern.
func (s *Store) RestoreLintPattern(id int64) error {
	const q = `UPDATE lint_patterns SET deleted_at = NULL WHERE id=? AND deleted_at IS NOT NULL`
	_, err := s.db.Exec(q, id)
	return err
}

// GetAllAntiPatterns returns anti-patterns for the admin browser with search,
// filter, pagination, and optional soft-deleted pattern inclusion.
func (s *Store) GetAllAntiPatterns(search, category string, includeDeleted bool, limit, offset int) ([]AntiPattern, int64, error) {
	var conditions []string
	var args []any

	if !includeDeleted {
		conditions = append(conditions, "deleted_at IS NULL")
	}
	if search != "" {
		pattern := "%" + escapeLike(search) + "%"
		conditions = append(conditions, "(pattern_id LIKE ? ESCAPE '\\' OR description LIKE ? ESCAPE '\\' OR dont_code LIKE ? ESCAPE '\\' OR do_code LIKE ? ESCAPE '\\')")
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if category != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, category)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM anti_patterns" + whereClause
	if err := s.db.QueryRow(countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count anti patterns: %w", err)
	}

	q := "SELECT id, pattern_id, description, dont_code, do_code, source, category, created_at, deleted_at FROM anti_patterns" +
		whereClause + " ORDER BY id DESC LIMIT ? OFFSET ?"
	queryArgs := append(args, limit, offset)

	rows, err := s.db.Query(q, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var aps []AntiPattern
	for rows.Next() {
		var ap AntiPattern
		var createdAt string
		var deletedAt sql.NullString
		if err := rows.Scan(&ap.ID, &ap.PatternID, &ap.Description, &ap.DontCode, &ap.DoCode,
			&ap.Source, &ap.Category, &createdAt, &deletedAt); err != nil {
			return nil, 0, err
		}
		ap.CreatedAt, _ = parseSQLiteTime(createdAt)
		if deletedAt.Valid {
			t, _ := parseSQLiteTime(deletedAt.String)
			ap.DeletedAt = &t
		}
		aps = append(aps, ap)
	}
	return aps, total, rows.Err()
}

// GetAntiPatternByID returns a single anti-pattern by ID (including soft-deleted ones).
func (s *Store) GetAntiPatternByID(id int64) (*AntiPattern, error) {
	const q = `
SELECT id, pattern_id, description, dont_code, do_code, source, category, created_at, deleted_at
FROM anti_patterns WHERE id=?`
	var ap AntiPattern
	var createdAt string
	var deletedAt sql.NullString
	err := s.db.QueryRow(q, id).Scan(&ap.ID, &ap.PatternID, &ap.Description, &ap.DontCode, &ap.DoCode,
		&ap.Source, &ap.Category, &createdAt, &deletedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ap.CreatedAt, _ = parseSQLiteTime(createdAt)
	if deletedAt.Valid {
		t, _ := parseSQLiteTime(deletedAt.String)
		ap.DeletedAt = &t
	}
	return &ap, nil
}

// SoftDeleteAntiPattern marks an anti-pattern as deleted.
func (s *Store) SoftDeleteAntiPattern(id int64) error {
	const q = `UPDATE anti_patterns SET deleted_at = CURRENT_TIMESTAMP WHERE id=? AND deleted_at IS NULL`
	_, err := s.db.Exec(q, id)
	return err
}

// RestoreAntiPattern clears the deleted_at flag on a soft-deleted anti-pattern.
func (s *Store) RestoreAntiPattern(id int64) error {
	const q = `UPDATE anti_patterns SET deleted_at = NULL WHERE id=? AND deleted_at IS NOT NULL`
	_, err := s.db.Exec(q, id)
	return err
}

// GetQualitySuggestions runs server-side analysis queries to identify patterns
// that may benefit from user attention.
func (s *Store) GetQualitySuggestions() ([]QualitySuggestion, error) {
	var suggestions []QualitySuggestion

	// 1. Patterns with empty do_code (no fix guidance).
	emptyRows, err := s.db.Query(`SELECT id, rule FROM lint_patterns WHERE do_code = '' AND deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("empty do_code query: %w", err)
	}
	defer emptyRows.Close()
	for emptyRows.Next() {
		var id int64
		var rule string
		if err := emptyRows.Scan(&id, &rule); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, QualitySuggestion{
			Type:        "empty_do_code",
			PatternIDs:  []int64{id},
			Description: fmt.Sprintf("Pattern %q has no do_code (fix example)", rule),
			Action:      "update",
		})
	}
	if err := emptyRows.Err(); err != nil {
		return nil, err
	}

	// 2. Low-frequency patterns (seen only once).
	lowRows, err := s.db.Query(`SELECT id, rule FROM lint_patterns WHERE frequency = 1 AND deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("low frequency query: %w", err)
	}
	defer lowRows.Close()
	for lowRows.Next() {
		var id int64
		var rule string
		if err := lowRows.Scan(&id, &rule); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, QualitySuggestion{
			Type:        "low_frequency",
			PatternIDs:  []int64{id},
			Description: fmt.Sprintf("Pattern %q has been seen only once", rule),
			Action:      "remove",
		})
	}
	if err := lowRows.Err(); err != nil {
		return nil, err
	}

	// 3. Duplicate dont_code patterns.
	dupRows, err := s.db.Query(`SELECT dont_code, GROUP_CONCAT(id) FROM lint_patterns WHERE deleted_at IS NULL GROUP BY dont_code HAVING COUNT(*) > 1`)
	if err != nil {
		return nil, fmt.Errorf("duplicate dont_code query: %w", err)
	}
	defer dupRows.Close()
	for dupRows.Next() {
		var dontCode, idsStr string
		if err := dupRows.Scan(&dontCode, &idsStr); err != nil {
			return nil, err
		}
		var ids []int64
		for _, part := range strings.Split(idsStr, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			var id int64
			if _, err := fmt.Sscan(part, &id); err == nil {
				ids = append(ids, id)
			}
		}
		if len(ids) > 1 {
			suggestions = append(suggestions, QualitySuggestion{
				Type:        "duplicate_dont_code",
				PatternIDs:  ids,
				Description: fmt.Sprintf("Multiple patterns share the same dont_code (%d patterns)", len(ids)),
				Action:      "merge",
			})
		}
	}
	if err := dupRows.Err(); err != nil {
		return nil, err
	}

	return suggestions, nil
}
