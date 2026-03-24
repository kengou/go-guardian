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
}

// AntiPattern represents a known anti-pattern stored in the database.
type AntiPattern struct {
	PatternID   string
	Description string
	DontCode    string
	DoCode      string
	Source      string
	Category    string
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

// PatternStats is an aggregate of pattern and scan statistics.
type PatternStats struct {
	TopLintPatterns   []LintPattern
	OWASPCounts       map[string]int64
	TotalLintPatterns int64
	TotalAntiPatterns int64
	RecentScans       []ScanHistory
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
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragma setup: %w", err)
	}

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

	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// runSchema executes the inlined DDL statements to create all tables and indexes.
func runSchema(db *sql.DB) error {
	_, err := db.Exec(schemaStatements)
	return err
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
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := fs.ReadFile(seedFiles, "seed/"+entry.Name())
		if err != nil {
			continue
		}
		// Execute the entire file in one call; the modernc sqlite driver accepts
		// multiple statements in a single Exec.
		if _, err := db.Exec(string(data)); err != nil {
			log.Printf("[go-guardian] warning: seed file %s execution error: %v", entry.Name(), err)
		}
	}
}

// InsertLintPattern upserts a lint pattern: on conflict it increments frequency
// and updates last_seen.
func (s *Store) InsertLintPattern(rule, fileGlob, dontCode, doCode, source string) error {
	const q = `
INSERT INTO lint_patterns (rule, file_glob, dont_code, do_code, source)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(rule, file_glob, dont_code) DO UPDATE SET
    frequency = frequency + 1,
    last_seen = CURRENT_TIMESTAMP,
    do_code   = excluded.do_code,
    source    = excluded.source`
	_, err := s.db.Exec(q, rule, fileGlob, dontCode, doCode, source)
	return err
}

// QueryPatterns returns lint patterns whose file_glob matches the given glob
// string (substring LIKE match), ordered by frequency descending, up to limit
// rows. The codeContext parameter is reserved for future semantic filtering.
func (s *Store) QueryPatterns(fileGlob, codeContext string, limit int) ([]LintPattern, error) {
	const q = `
SELECT id, rule, file_glob, dont_code, do_code, frequency, source, last_seen
FROM lint_patterns
WHERE file_glob LIKE ?
ORDER BY frequency DESC
LIMIT ?`
	rows, err := s.db.Query(q, "%"+fileGlob+"%", limit)
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

// InsertOWASPFinding upserts an OWASP finding: inserts on first encounter,
// increments frequency on subsequent calls for the same category/file_pattern/finding.
func (s *Store) InsertOWASPFinding(category, filePattern, finding, fixPattern string) error {
	const checkQ = `SELECT id FROM owasp_findings WHERE category=? AND file_pattern=? AND finding=?`
	var id int64
	err := s.db.QueryRow(checkQ, category, filePattern, finding).Scan(&id)
	if err == sql.ErrNoRows {
		const insertQ = `
INSERT INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (?, ?, ?, ?)`
		_, err = s.db.Exec(insertQ, category, filePattern, finding, fixPattern)
		return err
	}
	if err != nil {
		return err
	}
	const updateQ = `UPDATE owasp_findings SET frequency=frequency+1, last_seen=CURRENT_TIMESTAMP WHERE id=?`
	_, err = s.db.Exec(updateQ, id)
	return err
}

// QueryOWASPFindings returns OWASP findings whose file_pattern matches the
// given glob string (substring LIKE match), ordered by frequency descending,
// up to limit rows.
func (s *Store) QueryOWASPFindings(fileGlob string, limit int) ([]OWASPFinding, error) {
	const q = `
SELECT id, category, file_pattern, finding, fix_pattern, frequency
FROM owasp_findings
WHERE file_pattern LIKE ?
ORDER BY frequency DESC
LIMIT ?`
	rows, err := s.db.Query(q, "%"+fileGlob+"%", limit)
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
			`SELECT pattern_id, description, dont_code, do_code, source, category FROM anti_patterns`,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT pattern_id, description, dont_code, do_code, source, category FROM anti_patterns WHERE category=?`,
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
func (s *Store) UpsertVulnCache(module, cveID, severity, affected, fixed, description string) error {
	const q = `
INSERT INTO vuln_cache (module, cve_id, severity, affected_versions, fixed_version, description)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(module, cve_id) DO UPDATE SET
    severity          = excluded.severity,
    affected_versions = excluded.affected_versions,
    fixed_version     = excluded.fixed_version,
    description       = excluded.description,
    fetched_at        = CURRENT_TIMESTAMP`
	_, err := s.db.Exec(q, module, cveID, severity, affected, fixed, description)
	return err
}

// GetVulnCache retrieves all cached vulnerabilities for the given module.
func (s *Store) GetVulnCache(module string) ([]VulnEntry, error) {
	const q = `
SELECT module, cve_id, severity, affected_versions, fixed_version, description, fetched_at
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
			&e.FixedVersion, &e.Description, &fetchedAt); err != nil {
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

	// Total lint patterns count.
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM lint_patterns`).Scan(&stats.TotalLintPatterns); err != nil {
		return nil, fmt.Errorf("total lint patterns: %w", err)
	}

	// Total anti-patterns count.
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM anti_patterns`).Scan(&stats.TotalAntiPatterns); err != nil {
		return nil, fmt.Errorf("total anti patterns: %w", err)
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

// parseSQLiteTime parses the datetime formats that SQLite's CURRENT_TIMESTAMP
// may produce. Returns the zero time on parse failure (error is discarded by
// callers via the blank identifier).
func parseSQLiteTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		time.RFC3339,
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
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
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
CREATE INDEX IF NOT EXISTS idx_vuln_cache_module ON vuln_cache(module);
CREATE INDEX IF NOT EXISTS idx_scan_history_project ON scan_history(project);
CREATE INDEX IF NOT EXISTS idx_anti_patterns_category ON anti_patterns(category);
`
