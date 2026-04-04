package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// activityEntry is the JSON shape returned by the activity endpoint.
type activityEntry struct {
	ID            int64  `json:"id"`
	ToolName      string `json:"tool_name"`
	Agent         string `json:"agent"`
	ParamsSummary string `json:"params_summary"`
	DurationMS    int64  `json:"duration_ms"`
	Error         string `json:"error"`
	CreatedAt     string `json:"created_at"`
}

// handleActivity returns recent MCP tool invocations as a JSON array.
// Query params: tool, agent, limit (default 100), offset (default 0).
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	tool := r.URL.Query().Get("tool")
	agent := r.URL.Query().Get("agent")

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	reqs, err := s.store.GetMCPRequests(tool, agent, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build response slice; ensure we return [] not null when empty.
	entries := make([]activityEntry, 0, len(reqs))
	for _, req := range reqs {
		entries = append(entries, activityEntry{
			ID:            req.ID,
			ToolName:      req.ToolName,
			Agent:         req.Agent,
			ParamsSummary: req.ParamsSummary,
			DurationMS:    req.DurationMS,
			Error:         req.Error,
			CreatedAt:     req.CreatedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// ── Dashboard types ─────────────────────────────────────────────────────────

type dashboardResponse struct {
	TotalPatterns       int64                `json:"total_patterns"`
	TotalAntiPatterns   int64                `json:"total_anti_patterns"`
	RecentLearningCount int64                `json:"recent_learning_count"`
	OWASPCounts         map[string]int64     `json:"owasp_counts"`
	RecentScans         []scanEntry          `json:"recent_scans"`
	SessionInfo         *sessionInfoEntry    `json:"session_info"`
	TrendSummary        []trendSummaryEntry  `json:"trend_summary"`
}

type scanEntry struct {
	ScanType      string `json:"scan_type"`
	LastRun       string `json:"last_run"`
	FindingsCount int64  `json:"findings_count"`
}

type sessionInfoEntry struct {
	SessionID    string `json:"session_id"`
	FindingCount int    `json:"finding_count"`
}

type trendSummaryEntry struct {
	ScanType  string `json:"scan_type"`
	Direction string `json:"direction"`
}

// handleDashboard returns aggregated knowledge base statistics.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")

	stats, err := s.store.GetPatternStats(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	recentCount, err := s.store.RecentLearningCount(7)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := dashboardResponse{
		TotalPatterns:       stats.TotalLintPatterns,
		TotalAntiPatterns:   stats.TotalAntiPatterns,
		RecentLearningCount: recentCount,
		OWASPCounts:         stats.OWASPCounts,
		RecentScans:         make([]scanEntry, 0, len(stats.RecentScans)),
		TrendSummary:        make([]trendSummaryEntry, 0),
	}

	// Convert recent scans.
	for _, scan := range stats.RecentScans {
		resp.RecentScans = append(resp.RecentScans, scanEntry{
			ScanType:      scan.ScanType,
			LastRun:       scan.LastRun.Format(time.RFC3339),
			FindingsCount: scan.FindingsCount,
		})
	}

	// Session info (if session is active).
	if s.sessionID != "" {
		findings, ferr := s.store.GetSessionFindings(s.sessionID, "")
		if ferr == nil {
			resp.SessionInfo = &sessionInfoEntry{
				SessionID:    s.sessionID,
				FindingCount: len(findings),
			}
		}
	}

	// Trend summary from scan snapshots.
	allSnapshots, serr := s.store.GetAllScanSnapshots(project, 50)
	if serr == nil && len(allSnapshots) > 0 {
		byType := make(map[string][]db.ScanSnapshot)
		for _, ss := range allSnapshots {
			byType[ss.ScanType] = append(byType[ss.ScanType], ss)
		}
		// Sort scan types for deterministic output.
		types := make([]string, 0, len(byType))
		for t := range byType {
			types = append(types, t)
		}
		sort.Strings(types)

		for _, st := range types {
			dir := computeTrendDirection(byType[st])
			resp.TrendSummary = append(resp.TrendSummary, trendSummaryEntry{
				ScanType:  st,
				Direction: dir,
			})
		}
	}

	// Ensure non-null JSON arrays and maps.
	if resp.OWASPCounts == nil {
		resp.OWASPCounts = make(map[string]int64)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ── Trends types ────────────────────────────────────────────────────────────

type trendsResponse struct {
	Snapshots  []snapshotEntry     `json:"snapshots"`
	Directions []trendSummaryEntry `json:"directions"`
}

type snapshotEntry struct {
	ScanType      string `json:"scan_type"`
	FindingsCount int64  `json:"findings_count"`
	CreatedAt     string `json:"created_at"`
}

// handleTrends returns scan snapshot time-series data with direction indicators.
func (s *Server) handleTrends(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	scanType := r.URL.Query().Get("scan_type")

	var snapshots []db.ScanSnapshot
	var err error
	if scanType != "" {
		snapshots, err = s.store.GetScanSnapshots(scanType, project, 100)
	} else {
		snapshots, err = s.store.GetAllScanSnapshots(project, 100)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := trendsResponse{
		Snapshots:  make([]snapshotEntry, 0, len(snapshots)),
		Directions: make([]trendSummaryEntry, 0),
	}

	for _, ss := range snapshots {
		resp.Snapshots = append(resp.Snapshots, snapshotEntry{
			ScanType:      ss.ScanType,
			FindingsCount: ss.FindingsCount,
			CreatedAt:     ss.CreatedAt.Format(time.RFC3339),
		})
	}

	// Compute directions.
	byType := make(map[string][]db.ScanSnapshot)
	for _, ss := range snapshots {
		byType[ss.ScanType] = append(byType[ss.ScanType], ss)
	}
	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, st := range types {
		dir := computeTrendDirection(byType[st])
		resp.Directions = append(resp.Directions, trendSummaryEntry{
			ScanType:  st,
			Direction: dir,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ── Pattern management types ───────────────────────────────────────────────

// patternEntry is the unified JSON shape for both lint and anti-patterns
// returned by the list and detail endpoints.
type patternEntry struct {
	ID          int64   `json:"id"`
	Type        string  `json:"type"`
	Rule        string  `json:"rule,omitempty"`
	FileGlob    string  `json:"file_glob,omitempty"`
	PatternID   string  `json:"pattern_id,omitempty"`
	Description string  `json:"description,omitempty"`
	DontCode    string  `json:"dont_code"`
	DoCode      string  `json:"do_code"`
	Frequency   *int64  `json:"frequency,omitempty"`
	Source      string  `json:"source"`
	Category    string  `json:"category,omitempty"`
	LastSeen    *string `json:"last_seen,omitempty"`
	CreatedAt   string  `json:"created_at"`
	DeletedAt   *string `json:"deleted_at"`
}

type patternListResponse struct {
	Items   []patternEntry `json:"items"`
	Total   int64          `json:"total"`
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
}

type historyEntry struct {
	ID             int64  `json:"id"`
	Action         string `json:"action"`
	BeforeSnapshot string `json:"before_snapshot"`
	AfterSnapshot  string `json:"after_snapshot"`
	CreatedAt      string `json:"created_at"`
}

type suggestionEntry struct {
	Type        string  `json:"type"`
	PatternIDs  []int64 `json:"pattern_ids"`
	Description string  `json:"description"`
	Action      string  `json:"action"`
}

type patternDetailResponse struct {
	Pattern patternEntry   `json:"pattern"`
	History []historyEntry `json:"history"`
}

type updatePatternRequest struct {
	Type     string `json:"type"`
	DontCode string `json:"dont_code"`
	DoCode   string `json:"do_code"`
	Rule     string `json:"rule"`
	FileGlob string `json:"file_glob"`
}

// lintPatternToEntry converts a db.LintPattern to a patternEntry.
func lintPatternToEntry(p db.LintPattern) patternEntry {
	e := patternEntry{
		ID:        p.ID,
		Type:      "lint",
		Rule:      p.Rule,
		FileGlob:  p.FileGlob,
		DontCode:  p.DontCode,
		DoCode:    p.DoCode,
		Frequency: &p.Frequency,
		Source:    p.Source,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
	}
	ls := p.LastSeen.Format(time.RFC3339)
	e.LastSeen = &ls
	if p.DeletedAt != nil {
		da := p.DeletedAt.Format(time.RFC3339)
		e.DeletedAt = &da
	}
	return e
}

// antiPatternToEntry converts a db.AntiPattern to a patternEntry.
func antiPatternToEntry(p db.AntiPattern) patternEntry {
	e := patternEntry{
		ID:          p.ID,
		Type:        "anti",
		PatternID:   p.PatternID,
		Description: p.Description,
		DontCode:    p.DontCode,
		DoCode:      p.DoCode,
		Source:      p.Source,
		Category:    p.Category,
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
	}
	if p.DeletedAt != nil {
		da := p.DeletedAt.Format(time.RFC3339)
		e.DeletedAt = &da
	}
	return e
}

// historyFromDB converts db.PatternHistory entries to historyEntry slice.
func historyFromDB(entries []db.PatternHistory) []historyEntry {
	result := make([]historyEntry, 0, len(entries))
	for _, h := range entries {
		result = append(result, historyEntry{
			ID:             h.ID,
			Action:         h.Action,
			BeforeSnapshot: h.BeforeSnapshot,
			AfterSnapshot:  h.AfterSnapshot,
			CreatedAt:      h.CreatedAt.Format(time.RFC3339),
		})
	}
	return result
}

// handleListPatterns returns a paginated, filterable list of lint and/or anti-patterns.
func (s *Server) handleListPatterns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	source := q.Get("source")
	rule := q.Get("rule")
	typeFilter := q.Get("type")
	sortBy := q.Get("sort")
	includeDeleted := q.Get("include_deleted") == "true"

	page := 1
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	perPage := 50
	if v := q.Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = min(n, 200)
		}
	}

	if sortBy == "" {
		sortBy = "frequency"
	}

	offset := (page - 1) * perPage
	var items []patternEntry
	var total int64

	if typeFilter == "" || typeFilter == "lint" {
		lintPatterns, lintTotal, err := s.store.GetAllLintPatterns(search, source, rule, sortBy, includeDeleted, perPage, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		total += lintTotal
		for _, p := range lintPatterns {
			items = append(items, lintPatternToEntry(p))
		}
	}

	if typeFilter == "" || typeFilter == "anti" {
		category := q.Get("category")
		antiPatterns, antiTotal, err := s.store.GetAllAntiPatterns(search, category, includeDeleted, perPage, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		total += antiTotal
		for _, p := range antiPatterns {
			items = append(items, antiPatternToEntry(p))
		}
	}

	if items == nil {
		items = []patternEntry{}
	}

	resp := patternListResponse{
		Items:   items,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleGetPattern returns a single pattern by ID with its edit history.
func (s *Server) handleGetPattern(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	typeParam := r.URL.Query().Get("type")
	if typeParam != "lint" && typeParam != "anti" {
		http.Error(w, "type query parameter required (lint or anti)", http.StatusBadRequest)
		return
	}

	var entry patternEntry
	if typeParam == "lint" {
		p, err := s.store.GetLintPatternByID(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if p == nil {
			http.Error(w, "pattern not found", http.StatusNotFound)
			return
		}
		entry = lintPatternToEntry(*p)
	} else {
		p, err := s.store.GetAntiPatternByID(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if p == nil {
			http.Error(w, "pattern not found", http.StatusNotFound)
			return
		}
		entry = antiPatternToEntry(*p)
	}

	history, err := s.store.GetPatternHistory(typeParam, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := patternDetailResponse{
		Pattern: entry,
		History: historyFromDB(history),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleUpdatePattern updates a lint pattern's mutable fields and records history.
func (s *Server) handleUpdatePattern(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req updatePatternRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Type != "lint" {
		http.Error(w, "only lint patterns can be updated", http.StatusBadRequest)
		return
	}

	// Fetch current pattern as "before" snapshot.
	before, err := s.store.GetLintPatternByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if before == nil {
		http.Error(w, "pattern not found", http.StatusNotFound)
		return
	}
	beforeJSON, _ := json.Marshal(before)

	// Apply update.
	if err := s.store.UpdateLintPattern(id, req.DontCode, req.DoCode, req.Rule, req.FileGlob); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch updated pattern as "after" snapshot.
	after, err := s.store.GetLintPatternByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	afterJSON, _ := json.Marshal(after)

	// Record history.
	_ = s.store.InsertPatternHistory("lint", id, "edit", string(beforeJSON), string(afterJSON))

	entry := lintPatternToEntry(*after)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}

// handleDeletePattern soft-deletes a lint or anti-pattern and records history.
func (s *Server) handleDeletePattern(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	typeParam := r.URL.Query().Get("type")
	if typeParam != "lint" && typeParam != "anti" {
		http.Error(w, "type query parameter required (lint or anti)", http.StatusBadRequest)
		return
	}

	// Fetch current pattern as "before" snapshot.
	var beforeJSON []byte
	if typeParam == "lint" {
		p, err := s.store.GetLintPatternByID(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if p == nil {
			http.Error(w, "pattern not found", http.StatusNotFound)
			return
		}
		beforeJSON, _ = json.Marshal(p)
		if err := s.store.SoftDeleteLintPattern(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		p, err := s.store.GetAntiPatternByID(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if p == nil {
			http.Error(w, "pattern not found", http.StatusNotFound)
			return
		}
		beforeJSON, _ = json.Marshal(p)
		if err := s.store.SoftDeleteAntiPattern(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	_ = s.store.InsertPatternHistory(typeParam, id, "delete", string(beforeJSON), "{}")

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

// handleRestorePattern restores a soft-deleted pattern and records history.
func (s *Server) handleRestorePattern(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	typeParam := r.URL.Query().Get("type")
	if typeParam != "lint" && typeParam != "anti" {
		http.Error(w, "type query parameter required (lint or anti)", http.StatusBadRequest)
		return
	}

	if typeParam == "lint" {
		if err := s.store.RestoreLintPattern(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p, err := s.store.GetLintPatternByID(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		afterJSON, _ := json.Marshal(p)
		_ = s.store.InsertPatternHistory("lint", id, "restore", "{}", string(afterJSON))
	} else {
		if err := s.store.RestoreAntiPattern(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p, err := s.store.GetAntiPatternByID(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		afterJSON, _ := json.Marshal(p)
		_ = s.store.InsertPatternHistory("anti", id, "restore", "{}", string(afterJSON))
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

// handlePatternHistory returns the edit history for a specific pattern.
func (s *Server) handlePatternHistory(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	typeParam := r.URL.Query().Get("type")
	if typeParam != "lint" && typeParam != "anti" {
		http.Error(w, "type query parameter required (lint or anti)", http.StatusBadRequest)
		return
	}

	history, err := s.store.GetPatternHistory(typeParam, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	entries := historyFromDB(history)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// handleSuggestions returns quality improvement suggestions for patterns.
func (s *Server) handleSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, err := s.store.GetQualitySuggestions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	entries := make([]suggestionEntry, 0, len(suggestions))
	for _, sg := range suggestions {
		ids := sg.PatternIDs
		if ids == nil {
			ids = []int64{}
		}
		entries = append(entries, suggestionEntry{
			Type:        sg.Type,
			PatternIDs:  ids,
			Description: sg.Description,
			Action:      sg.Action,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// computeTrendDirection determines direction from the most recent snapshots.
// Snapshots are ordered most-recent first.
func computeTrendDirection(snapshots []db.ScanSnapshot) string {
	if len(snapshots) < 2 {
		return "insufficient_data"
	}
	limit := min(len(snapshots), 3)
	recent := snapshots[:limit]

	improving := true
	degrading := true
	for i := 0; i < len(recent)-1; i++ {
		if recent[i].FindingsCount >= recent[i+1].FindingsCount {
			improving = false
		}
		if recent[i].FindingsCount <= recent[i+1].FindingsCount {
			degrading = false
		}
	}

	switch {
	case improving:
		return "improving"
	case degrading:
		return "degrading"
	default:
		return "stable"
	}
}

// ── Domain browser handlers ────────────────────────────────────────────────

// sessionFindingEntry is the JSON shape for session findings.
type sessionFindingEntry struct {
	ID          int64  `json:"id"`
	Agent       string `json:"agent"`
	FindingType string `json:"finding_type"`
	FilePath    string `json:"file_path"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	CreatedAt   string `json:"created_at"`
}

// handleSessionFindings returns session findings scoped to the current session.
// Query params: agent, severity, finding_type, file_path.
func (s *Server) handleSessionFindings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	agent := q.Get("agent")
	severity := q.Get("severity")
	findingType := q.Get("finding_type")
	filePath := q.Get("file_path")

	// No session ID means no findings.
	if s.sessionID == "" {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
		return
	}

	var findings []db.SessionFinding
	var err error

	if filePath != "" {
		findings, err = s.store.GetSessionFindingsByFile(s.sessionID, filePath)
	} else {
		findings, err = s.store.GetSessionFindings(s.sessionID, agent)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Post-filter by severity and finding_type if set.
	entries := make([]sessionFindingEntry, 0, len(findings))
	for _, f := range findings {
		if severity != "" && !strings.EqualFold(f.Severity, severity) {
			continue
		}
		if findingType != "" && f.FindingType != findingType {
			continue
		}
		entries = append(entries, sessionFindingEntry{
			ID:          f.ID,
			Agent:       f.Agent,
			FindingType: f.FindingType,
			FilePath:    f.FilePath,
			Description: f.Description,
			Severity:    f.Severity,
			CreatedAt:   f.CreatedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// ── OWASP types ────────────────────────────────────────────────────────────

type owaspCategoryEntry struct {
	Category     string              `json:"category"`
	FindingCount int                 `json:"finding_count"`
	Findings     []owaspFindingEntry `json:"findings"`
}

type owaspFindingEntry struct {
	ID          int64  `json:"id"`
	FilePattern string `json:"file_pattern"`
	Finding     string `json:"finding"`
	FixPattern  string `json:"fix_pattern"`
	Frequency   int64  `json:"frequency"`
}

type owaspResponse struct {
	Categories []owaspCategoryEntry `json:"categories"`
}

// handleOWASP returns OWASP findings grouped by category.
// Query params: category (optional filter).
func (s *Server) handleOWASP(w http.ResponseWriter, r *http.Request) {
	categoryFilter := r.URL.Query().Get("category")

	findings, err := s.store.QueryOWASPFindings("", 1000)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Group by category.
	grouped := make(map[string][]owaspFindingEntry)
	for _, f := range findings {
		grouped[f.Category] = append(grouped[f.Category], owaspFindingEntry{
			ID:          f.ID,
			FilePattern: f.FilePattern,
			Finding:     f.Finding,
			FixPattern:  f.FixPattern,
			Frequency:   f.Frequency,
		})
	}

	// Build sorted categories.
	cats := make([]string, 0, len(grouped))
	for c := range grouped {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	categories := make([]owaspCategoryEntry, 0, len(cats))
	for _, c := range cats {
		if categoryFilter != "" && c != categoryFilter {
			continue
		}
		flist := grouped[c]
		// Sort findings by frequency DESC.
		sort.Slice(flist, func(i, j int) bool {
			return flist[i].Frequency > flist[j].Frequency
		})
		categories = append(categories, owaspCategoryEntry{
			Category:     c,
			FindingCount: len(flist),
			Findings:     flist,
		})
	}

	resp := owaspResponse{Categories: categories}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ── Vulnerabilities types ──────────────────────────────────────────────────

type vulnModuleEntry struct {
	Module          string               `json:"module"`
	Decision        *vulnDecisionEntry   `json:"decision"`
	Vulnerabilities []vulnDetailEntry    `json:"vulnerabilities"`
}

type vulnDecisionEntry struct {
	Decision  string `json:"decision"`
	Reason    string `json:"reason"`
	CVECount  int64  `json:"cve_count"`
	CheckedAt string `json:"checked_at"`
}

type vulnDetailEntry struct {
	CVEID            string `json:"cve_id"`
	Severity         string `json:"severity"`
	AffectedVersions string `json:"affected_versions"`
	FixedVersion     string `json:"fixed_version"`
	Description      string `json:"description"`
	Source           string `json:"source"`
	FetchedAt        string `json:"fetched_at"`
}

type vulnResponse struct {
	Modules []vulnModuleEntry `json:"modules"`
}

// handleVulnerabilities returns vulnerabilities grouped by module with decisions.
// Query params: module (optional filter), severity (optional filter).
func (s *Server) handleVulnerabilities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	moduleFilter := q.Get("module")
	severityFilter := q.Get("severity")

	vulns, err := s.store.GetAllVulnEntries(500)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	decisions, err := s.store.GetAllDepDecisions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Index decisions by module.
	decisionMap := make(map[string]db.DepDecision, len(decisions))
	for _, d := range decisions {
		decisionMap[d.Module] = d
	}

	// Group vulns by module.
	grouped := make(map[string][]vulnDetailEntry)
	for _, v := range vulns {
		if severityFilter != "" && !strings.EqualFold(v.Severity, severityFilter) {
			continue
		}
		source := v.Source
		if source == "" {
			source = "go-vuln"
		}
		grouped[v.Module] = append(grouped[v.Module], vulnDetailEntry{
			CVEID:            v.CVEID,
			Severity:         v.Severity,
			AffectedVersions: v.AffectedVersions,
			FixedVersion:     v.FixedVersion,
			Description:      v.Description,
			Source:           source,
			FetchedAt:        v.FetchedAt.Format(time.RFC3339),
		})
	}

	// Build sorted modules list.
	mods := make([]string, 0, len(grouped))
	for m := range grouped {
		mods = append(mods, m)
	}
	sort.Strings(mods)

	modules := make([]vulnModuleEntry, 0, len(mods))
	for _, m := range mods {
		if moduleFilter != "" && m != moduleFilter {
			continue
		}
		entry := vulnModuleEntry{
			Module:          m,
			Vulnerabilities: grouped[m],
		}
		if d, ok := decisionMap[m]; ok {
			entry.Decision = &vulnDecisionEntry{
				Decision:  d.Decision,
				Reason:    d.Reason,
				CVECount:  d.CVECount,
				CheckedAt: d.CheckedAt.Format(time.RFC3339),
			}
		}
		modules = append(modules, entry)
	}

	resp := vulnResponse{Modules: modules}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handlePrefetchStatus returns the current CVE prefetch progress.
func (s *Server) handlePrefetchStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.prefetchStatus == nil {
		json.NewEncoder(w).Encode(map[string]string{"phase": "idle"})
		return
	}
	snap := s.prefetchStatus.Snapshot()
	json.NewEncoder(w).Encode(map[string]any{
		"phase":         snap.Phase,
		"source":        snap.Source,
		"progress":      snap.Progress,
		"total":         snap.Total,
		"cves_found":    snap.CVEsFound,
		"cves_enriched": snap.CVEsEnriched,
		"last_refresh":  snap.LastRefresh,
		"error":         snap.Error,
	})
}

// ── Renovate types ─────────────────────────────────────────────────────────

type renovatePreferenceEntry struct {
	ID          int64  `json:"id"`
	Category    string `json:"category"`
	Description string `json:"description"`
	DontConfig  string `json:"dont_config"`
	DoConfig    string `json:"do_config"`
	Frequency   int64  `json:"frequency"`
	Source      string `json:"source"`
}

type renovateRuleEntry struct {
	ID          int64  `json:"id"`
	RuleID      string `json:"rule_id"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Description string `json:"description"`
	DontConfig  string `json:"dont_config"`
	DoConfig    string `json:"do_config"`
	Severity    string `json:"severity"`
}

type configScoreEntry struct {
	ID             int64  `json:"id"`
	ConfigPath     string `json:"config_path"`
	Score          int    `json:"score"`
	FindingsCount  int    `json:"findings_count"`
	FindingsDetail string `json:"findings_detail"`
	CreatedAt      string `json:"created_at"`
}

type renovateResponse struct {
	Preferences  []renovatePreferenceEntry `json:"preferences"`
	Rules        []renovateRuleEntry       `json:"rules"`
	ConfigScores []configScoreEntry        `json:"config_scores"`
}

// handleRenovate returns Renovate preferences, rules, and config score history.
// Query params: category (optional filter).
func (s *Server) handleRenovate(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	prefs, err := s.store.QueryRenovatePreferences(category, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rules, err := s.store.QueryRenovateRules(category)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	scores, err := s.store.GetRecentConfigScores(20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build response with guaranteed non-null arrays.
	resp := renovateResponse{
		Preferences:  make([]renovatePreferenceEntry, 0, len(prefs)),
		Rules:        make([]renovateRuleEntry, 0, len(rules)),
		ConfigScores: make([]configScoreEntry, 0, len(scores)),
	}

	for _, p := range prefs {
		resp.Preferences = append(resp.Preferences, renovatePreferenceEntry{
			ID:          p.ID,
			Category:    p.Category,
			Description: p.Description,
			DontConfig:  p.DontConfig,
			DoConfig:    p.DoConfig,
			Frequency:   p.Frequency,
			Source:      p.Source,
		})
	}

	for _, r := range rules {
		resp.Rules = append(resp.Rules, renovateRuleEntry{
			ID:          r.ID,
			RuleID:      r.RuleID,
			Category:    r.Category,
			Title:       r.Title,
			Description: r.Description,
			DontConfig:  r.DontConfig,
			DoConfig:    r.DoConfig,
			Severity:    r.Severity,
		})
	}

	for _, cs := range scores {
		resp.ConfigScores = append(resp.ConfigScores, configScoreEntry{
			ID:             cs.ID,
			ConfigPath:     cs.ConfigPath,
			Score:          cs.Score,
			FindingsCount:  cs.FindingsCount,
			FindingsDetail: cs.FindingsDetail,
			CreatedAt:      cs.CreatedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
