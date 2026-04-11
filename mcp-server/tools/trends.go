package tools

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// RunGetHealthTrends returns a formatted health-trends report for the given
// project (or all projects if empty). scanType optionally filters to a single
// scan type (owasp, vuln, lint, full). This is the CLI-callable surface for
// get_health_trends and is invoked by the go-guardian scan subcommand.
func RunGetHealthTrends(store *db.Store, project, scanType string) (string, error) {
	var snapshots []db.ScanSnapshot
	var err error

	if scanType != "" {
		snapshots, err = store.GetScanSnapshots(scanType, project, 30)
	} else {
		snapshots, err = store.GetAllScanSnapshots(project, 50)
	}
	if err != nil {
		return "", fmt.Errorf("query snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		return "No scan history available yet. Run /go to start building trend data.", nil
	}

	var sb strings.Builder
	sb.WriteString("Health Trends")
	if project != "" {
		fmt.Fprintf(&sb, " — %s", project)
	}
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("═", 40))
	sb.WriteString("\n\n")

	// Group snapshots by scan_type.
	byType := groupByType(snapshots)

	// Compute per-type trends.
	types := sortedKeys(byType)
	for _, st := range types {
		typeSnapshots := byType[st]
		dir := computeDirection(typeSnapshots)
		fmt.Fprintf(&sb, "%s: %s", st, dir)

		if len(typeSnapshots) >= 2 {
			delta := typeSnapshots[0].FindingsCount - typeSnapshots[1].FindingsCount
			if delta > 0 {
				fmt.Fprintf(&sb, " (+%d since last scan)", delta)
			} else if delta < 0 {
				fmt.Fprintf(&sb, " (%d since last scan)", delta)
			}
		}
		sb.WriteString("\n")

		// Show recent counts.
		if len(typeSnapshots) > 1 {
			sb.WriteString("  History: ")
			limit := len(typeSnapshots)
			if limit > 5 {
				limit = 5
			}
			for i := limit - 1; i >= 0; i-- {
				if i < limit-1 {
					sb.WriteString(" → ")
				}
				fmt.Fprintf(&sb, "%d", typeSnapshots[i].FindingsCount)
			}
			sb.WriteString("\n")
		}
	}

	// Compute overall direction from all types combined.
	overall := computeOverallDirection(byType)
	fmt.Fprintf(&sb, "\nOverall: %s\n", overall)

	// Recurring categories from findings_detail JSON.
	recurring := computeRecurring(snapshots)
	if len(recurring) > 0 {
		sb.WriteString("\nTop Recurring Issues:\n")
		for i, r := range recurring {
			fmt.Fprintf(&sb, "  %d. %s (in %d scans)\n", i+1, r.name, r.count)
		}
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}

// groupByType groups snapshots by scan_type.
func groupByType(snapshots []db.ScanSnapshot) map[string][]db.ScanSnapshot {
	m := make(map[string][]db.ScanSnapshot)
	for _, ss := range snapshots {
		m[ss.ScanType] = append(m[ss.ScanType], ss)
	}
	return m
}

// computeDirection determines the trend from the most recent snapshots.
// Snapshots are ordered most-recent first.
func computeDirection(snapshots []db.ScanSnapshot) string {
	if len(snapshots) < 2 {
		return "no trend (insufficient data)"
	}

	// Use up to 3 most recent.
	limit := len(snapshots)
	if limit > 3 {
		limit = 3
	}
	recent := snapshots[:limit]

	// Check monotonic direction (recent[0] is newest).
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
		// Newer has more findings → degrading (improving means fewer findings).
		// Wait — recent[0] is newest. If newest >= older, that's degrading.
		// improving = true means recent[i] < recent[i+1] for all i.
		// i.e., newest < oldest → findings are decreasing → improving.
		return "IMPROVING"
	case degrading:
		// recent[i] > recent[i+1] for all i → newest > oldest → degrading.
		return "DEGRADING"
	default:
		return "STABLE"
	}
}

// computeOverallDirection combines per-type directions.
func computeOverallDirection(byType map[string][]db.ScanSnapshot) string {
	improving := 0
	degrading := 0
	for _, snapshots := range byType {
		dir := computeDirection(snapshots)
		switch dir {
		case "IMPROVING":
			improving++
		case "DEGRADING":
			degrading++
		}
	}
	switch {
	case degrading > improving:
		return "DEGRADING"
	case improving > degrading:
		return "IMPROVING"
	default:
		return "STABLE"
	}
}

type recurringIssue struct {
	name  string
	count int
}

// computeRecurring parses findings_detail JSON blobs and counts category
// occurrences across scans. Returns top 5.
func computeRecurring(snapshots []db.ScanSnapshot) []recurringIssue {
	catCounts := make(map[string]int)

	for _, ss := range snapshots {
		if ss.FindingsDetail == "" || ss.FindingsDetail == "{}" {
			continue
		}
		var detail map[string]interface{}
		if err := json.Unmarshal([]byte(ss.FindingsDetail), &detail); err != nil {
			continue
		}
		// Look for "categories" key with map[string]count.
		cats, ok := detail["categories"]
		if !ok {
			continue
		}
		catsMap, ok := cats.(map[string]interface{})
		if !ok {
			continue
		}
		for cat := range catsMap {
			catCounts[cat]++
		}
	}

	var result []recurringIssue
	for cat, cnt := range catCounts {
		result = append(result, recurringIssue{name: cat, count: cnt})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].count > result[j].count
	})
	if len(result) > 5 {
		result = result[:5]
	}
	return result
}

func sortedKeys(m map[string][]db.ScanSnapshot) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
