package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
)

const cacheMaxAge = 24 * time.Hour

// moduleResult holds the analysis output for a single Go module.
type moduleResult struct {
	module        string
	status        string // e.g. "PREFER", "CHECK LATEST", "AVOID", "UNKNOWN"
	cves          []db.VulnEntry
	stale         bool // true when cache exists but is older than cacheMaxAge
	noCache       bool // true when no cache data exists at all
	priorDecision *db.DepDecision
}

// RunCheckDeps analyses the given Go modules for known vulnerabilities and
// returns a formatted report. Side effects (dep_decisions upsert, scan
// snapshot insert) match the MCP handler. An input slice whose entries are
// all empty after trimming returns a neutral
// "no valid module paths provided" message with a nil error.
func RunCheckDeps(store *db.Store, modules []string) (string, error) {
	// Normalize + drop empty entries.
	normalized := make([]string, 0, len(modules))
	for _, m := range modules {
		if s := strings.TrimSpace(m); s != "" {
			normalized = append(normalized, s)
		}
	}
	if len(normalized) == 0 {
		return "Dependency Analysis:\n\n(no valid module paths provided)", nil
	}

	results, err := analyseModules(store, normalized)
	if err != nil {
		return "", fmt.Errorf("analysis error: %w", err)
	}

	for _, r := range results {
		if r.noCache || r.stale {
			continue
		}
		decision, reason := decisionForResult(r)
		if uErr := store.UpsertDepDecision(r.module, decision, reason, len(r.cves)); uErr != nil {
			_ = uErr
		}
	}

	totalCVEs := 0
	for _, r := range results {
		totalCVEs += len(r.cves)
	}
	_ = store.InsertScanSnapshot("vuln", "", totalCVEs, buildVulnSnapshotDetail(results))

	return formatResults(results), nil
}

// RegisterCheckDeps registers the check_deps MCP tool with the given server.
func RegisterCheckDeps(s ToolRegistrar, store *db.Store) {
	tool := mcp.NewTool(
		"check_deps",
		mcp.WithDescription(
			"Analyse Go module dependencies for known vulnerabilities using cached CVE data. "+
				"Accepts a list of module paths and returns a status (PREFER / CHECK LATEST / AVOID / UNKNOWN) "+
				"for each one, along with any cached CVE details and prior dependency decisions. "+
				"When no cached data is available, advises using the gateway vuln API tools to fetch live data.",
		),
		mcp.WithArray("modules",
			mcp.Required(),
			mcp.Description("Go module paths to check, e.g. [\"github.com/gorilla/mux\", \"github.com/gin-gonic/gin\"]"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["modules"].([]interface{})
		if len(raw) == 0 {
			return mcp.NewToolResultText("Dependency Analysis:\n\n(no modules provided)"), nil
		}

		modules := make([]string, 0, len(raw))
		for _, v := range raw {
			if modPath, ok := v.(string); ok {
				modules = append(modules, modPath)
			}
		}

		result, err := RunCheckDeps(store, modules)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})
}

// analyseModules queries the store for each module and computes recommendations.
func analyseModules(store *db.Store, modules []string) ([]moduleResult, error) {
	results := make([]moduleResult, 0, len(modules))
	for _, mod := range modules {
		r, err := analyseModule(store, mod)
		if err != nil {
			return nil, fmt.Errorf("module %q: %w", mod, err)
		}
		results = append(results, r)
	}
	return results, nil
}

// analyseModule performs the cache lookup and recommendation computation for one module.
func analyseModule(store *db.Store, module string) (moduleResult, error) {
	r := moduleResult{module: module}

	entries, err := store.GetVulnCache(module)
	if err != nil {
		return r, fmt.Errorf("GetVulnCache: %w", err)
	}

	decision, err := store.GetDepDecision(module)
	if err != nil {
		return r, fmt.Errorf("GetDepDecision: %w", err)
	}
	r.priorDecision = decision

	if len(entries) == 0 {
		r.noCache = true
		r.status = "UNKNOWN"
		return r, nil
	}

	// Check freshness: use the oldest fetched_at across all entries for the module.
	oldest := entries[0].FetchedAt
	for _, e := range entries[1:] {
		if e.FetchedAt.Before(oldest) {
			oldest = e.FetchedAt
		}
	}
	if time.Since(oldest) > cacheMaxAge {
		r.stale = true
		r.cves = entries
		r.status = "UNKNOWN"
		return r, nil
	}

	r.cves = entries
	r.status = computeStatus(entries)
	return r, nil
}

// computeStatus derives a status string from a set of CVE entries.
func computeStatus(cves []db.VulnEntry) string {
	if len(cves) == 0 {
		return "PREFER"
	}

	// Check for 3+ CVEs or any unfixed CRITICAL.
	unfixedCritical := false
	for _, c := range cves {
		if strings.EqualFold(c.Severity, "CRITICAL") && strings.TrimSpace(c.FixedVersion) == "" {
			unfixedCritical = true
			break
		}
	}
	if len(cves) >= 3 || unfixedCritical {
		return "AVOID"
	}

	// 1-2 CVEs: AVOID if any unfixed, CHECK LATEST if all have a fixed version.
	for _, c := range cves {
		if strings.TrimSpace(c.FixedVersion) == "" {
			return "AVOID"
		}
	}
	return "CHECK LATEST"
}

// decisionForResult maps a moduleResult to the (decision, reason) pair persisted in dep_decisions.
func decisionForResult(r moduleResult) (decision, reason string) {
	switch r.status {
	case "PREFER":
		return "prefer", "no known vulnerabilities"
	case "CHECK LATEST":
		return "check", fmt.Sprintf("%d CVE(s) exist but fixed versions are available", len(r.cves))
	case "AVOID":
		return "avoid", fmt.Sprintf("%d known CVE(s), significant vulnerability history", len(r.cves))
	default:
		return "unknown", "no cache data — run vuln scan to check"
	}
}

// formatResults renders all module results as a human-readable text block.
func formatResults(results []moduleResult) string {
	var sb strings.Builder
	sb.WriteString("Dependency Analysis:\n")

	for _, r := range results {
		sb.WriteString("\n")
		sb.WriteString(r.module)
		sb.WriteString("\n")

		// Status line.
		switch r.status {
		case "PREFER":
			sb.WriteString("  Status: PREFER — no known vulnerabilities\n")
		case "CHECK LATEST":
			sb.WriteString(fmt.Sprintf("  Status: CHECK LATEST — %d CVE(s) exist but fixed versions available\n", len(r.cves)))
		case "AVOID":
			sb.WriteString(fmt.Sprintf("  Status: AVOID — %d known CVE(s)\n", len(r.cves)))
		case "UNKNOWN":
			if r.stale {
				sb.WriteString("  Status: UNKNOWN — cached data is stale (>24h); re-run vuln scan for live data\n")
			} else {
				sb.WriteString("  Status: UNKNOWN — no cached data; run vuln scan to check\n")
			}
		}

		// CVE detail lines.
		if len(r.cves) > 0 && !r.noCache {
			cveStrs := make([]string, 0, len(r.cves))
			for _, c := range r.cves {
				fixed := "unfixed"
				if strings.TrimSpace(c.FixedVersion) != "" {
					fixed = "fixed: " + c.FixedVersion
				}
				cveStrs = append(cveStrs, fmt.Sprintf("%s (%s, %s)", c.CVEID, c.Severity, fixed))
			}
			sb.WriteString("  CVEs: ")
			sb.WriteString(strings.Join(cveStrs, ", "))
			sb.WriteString("\n")
		}

		// Recommendation.
		sb.WriteString("  Recommendation: ")
		sb.WriteString(recommendationFor(r))
		sb.WriteString("\n")

		// Prior decision.
		sb.WriteString("  Prior decision: ")
		if r.priorDecision == nil {
			sb.WriteString("none")
		} else {
			sb.WriteString(fmt.Sprintf("%s — %s (checked %s)",
				r.priorDecision.Decision,
				r.priorDecision.Reason,
				r.priorDecision.CheckedAt.Format("2006-01-02"),
			))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// recommendationFor returns a one-line recommendation string for a moduleResult.
func recommendationFor(r moduleResult) string {
	switch r.status {
	case "PREFER":
		return "Safe to use; no known vulnerabilities in cache."
	case "CHECK LATEST":
		// Find the highest fixed version across all CVEs.
		var fixedVersions []string
		for _, c := range r.cves {
			if v := strings.TrimSpace(c.FixedVersion); v != "" {
				fixedVersions = append(fixedVersions, v)
			}
		}
		if len(fixedVersions) > 0 {
			return fmt.Sprintf("Pin to %s or later to resolve known CVE(s).", fixedVersions[len(fixedVersions)-1])
		}
		return "Upgrade to the latest release to resolve known CVE(s)."
	case "AVOID":
		return "Significant vulnerability history; consider an alternative or consult the security team."
	default:
		return "Use the gateway vuln API tools to fetch live vulnerability data for this module."
	}
}

// buildVulnSnapshotDetail creates a JSON detail blob from dep analysis results,
// grouping CVE counts by severity.
func buildVulnSnapshotDetail(results []moduleResult) string {
	cats := make(map[string]int)
	for _, r := range results {
		for _, c := range r.cves {
			cats[c.Severity]++
		}
	}
	detail := map[string]interface{}{"categories": cats}
	b, _ := json.Marshal(detail)
	return string(b)
}

// ParseGoMod reads a go.mod file at path and returns all require module paths
// (without version strings). Blank lines and comment lines are ignored.
func ParseGoMod(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open go.mod: %w", err)
	}
	defer f.Close()

	var modules []string
	inRequireBlock := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments.
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Single-line require: require github.com/foo/bar v1.2.3
		if strings.HasPrefix(line, "require ") && !strings.HasSuffix(line, "(") {
			parts := strings.Fields(line)
			// parts[0] = "require", parts[1] = module path, parts[2] = version
			if len(parts) >= 2 {
				mod := parts[1]
				// Strip inline comment if present.
				if idx := strings.Index(mod, "//"); idx >= 0 {
					mod = strings.TrimSpace(mod[:idx])
				}
				if mod != "" {
					modules = append(modules, mod)
				}
			}
			continue
		}

		// Opening of a require block.
		if line == "require (" || strings.HasPrefix(line, "require(") {
			inRequireBlock = true
			continue
		}

		// Closing of a require block.
		if inRequireBlock && line == ")" {
			inRequireBlock = false
			continue
		}

		// Inside a require block: "github.com/foo/bar v1.2.3 // indirect"
		if inRequireBlock {
			parts := strings.Fields(line)
			if len(parts) >= 1 && !strings.HasPrefix(parts[0], "//") {
				mod := parts[0]
				modules = append(modules, mod)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan go.mod: %w", err)
	}
	return modules, nil
}
