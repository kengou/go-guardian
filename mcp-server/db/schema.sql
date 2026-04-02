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
