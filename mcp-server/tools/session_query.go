package tools

import (
	"fmt"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// RunGetSessionFindings queries cross-agent session findings with optional
// filters on agent, file path (substring), and finding type (case-insensitive).
// Returns a human-readable block or "No active session..." if sessionID is
// empty. The returned error is non-nil only on store query failures.
func RunGetSessionFindings(store *db.Store, sessionID, agent, filePath, findingType string) (string, error) {
	if sessionID == "" {
		return "No active session — session findings are only available during a /go scan.", nil
	}

	agent = strings.TrimSpace(agent)
	filePath = strings.TrimSpace(filePath)
	findingType = strings.TrimSpace(findingType)

	var findings []db.SessionFinding
	var err error

	if filePath != "" {
		findings, err = store.GetSessionFindingsByFile(sessionID, filePath)
	} else {
		findings, err = store.GetSessionFindings(sessionID, agent)
	}
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}

	if findingType != "" {
		filtered := findings[:0]
		for _, f := range findings {
			if strings.EqualFold(f.FindingType, findingType) {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}

	if len(findings) == 0 {
		return "No session findings reported yet.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Session Findings (%d)\n", len(findings))
	sb.WriteString(strings.Repeat("─", 30))
	sb.WriteString("\n")

	for _, f := range findings {
		fmt.Fprintf(&sb, "\n[%s] %s — %s", f.Severity, f.Agent, f.FindingType)
		if f.FilePath != "" {
			fmt.Fprintf(&sb, " (%s)", f.FilePath)
		}
		fmt.Fprintf(&sb, "\n  %s\n", f.Description)
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}

