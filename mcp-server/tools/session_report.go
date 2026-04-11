package tools

import (
	"fmt"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// RunReportFinding records a cross-agent session finding and returns a
// human-readable acknowledgment. sessionID must be non-empty; callers of the
// CLI variant will read it from .go-guardian/session-id. severity is
// normalized to CRITICAL/HIGH/MEDIUM/LOW; unknown or empty values fall back
// to MEDIUM.
func RunReportFinding(store *db.Store, sessionID, agent, findingType, filePath, description, severity string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("no active session — findings require a session ID")
	}

	agent = strings.TrimSpace(agent)
	findingType = strings.TrimSpace(findingType)
	description = strings.TrimSpace(description)

	if agent == "" || findingType == "" || description == "" {
		return "", fmt.Errorf("agent, finding_type, and description are required")
	}

	filePath = strings.TrimSpace(filePath)
	severity = strings.ToUpper(strings.TrimSpace(severity))
	if severity == "" {
		severity = "MEDIUM"
	}
	switch severity {
	case "CRITICAL", "HIGH", "MEDIUM", "LOW":
	default:
		severity = "MEDIUM"
	}

	id, err := store.InsertSessionFinding(sessionID, agent, findingType, filePath, description, severity)
	if err != nil {
		return "", fmt.Errorf("store: %w", err)
	}

	return fmt.Sprintf("Finding #%d recorded (%s/%s: %s)", id, agent, severity, findingType), nil
}

