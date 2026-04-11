package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// renovateSeverityDeduction maps severity levels to the score penalty per finding.
var renovateSeverityDeduction = map[string]int{
	"CRITICAL": 10,
	"WARN":     5,
	"INFO":     2,
}

// renovateSeverityOrder defines display ordering (CRITICAL first).
var renovateSeverityOrder = []string{"CRITICAL", "WARN", "INFO"}

// renovateFinding represents a single analysis finding.
type renovateFinding struct {
	RuleID      string `json:"rule_id"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// RunAnalyzeRenovateConfig analyzes a Renovate configuration file against the rule
// database. It is the package-level entry point used by the CLI.
func RunAnalyzeRenovateConfig(store *db.Store, configPath string) (string, error) {
	// Read and parse config.
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("cannot read config file: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("invalid JSON in config: %w", err)
	}

	// Query rules and preferences.
	rules, err := store.QueryRenovateRules("")
	if err != nil {
		return "", fmt.Errorf("query rules: %w", err)
	}

	// Preferences are queried for potential future use (e.g., bonus scoring),
	// but the core analysis is rule-driven.
	_, err = store.QueryRenovatePreferences("", 100)
	if err != nil {
		return "", fmt.Errorf("query preferences: %w", err)
	}

	// Evaluate each rule against the config.
	var findings []renovateFinding
	for _, rule := range rules {
		if f, violated := evaluateRule(rule, config); violated {
			findings = append(findings, f)
		}
	}

	// Compute score.
	score := 100
	for _, f := range findings {
		if d, ok := renovateSeverityDeduction[f.Severity]; ok {
			score -= d
		}
	}
	if score < 0 {
		score = 0
	}

	// Group findings by severity.
	grouped := make(map[string][]renovateFinding)
	for _, f := range findings {
		grouped[f.Severity] = append(grouped[f.Severity], f)
	}

	// Format output.
	var out strings.Builder
	fmt.Fprintf(&out, "=== Renovate Config Analysis: %s ===\n", configPath)
	fmt.Fprintf(&out, "Score: %d/100\n", score)

	for _, sev := range renovateSeverityOrder {
		fs := grouped[sev]
		if len(fs) == 0 {
			continue
		}
		noun := "findings"
		if len(fs) == 1 {
			noun = "finding"
		}
		fmt.Fprintf(&out, "\n%s (%d %s):\n", sev, len(fs), noun)
		for _, f := range fs {
			fmt.Fprintf(&out, "  [%s] %s — %s\n", f.RuleID, f.Title, f.Description)
		}
	}

	// Persist score.
	findingsJSON, _ := json.Marshal(findings)
	if err := store.InsertConfigScore(configPath, score, len(findings), string(findingsJSON)); err != nil {
		fmt.Fprintf(&out, "\nWarning: failed to save score: %v\n", err)
	} else {
		fmt.Fprintln(&out, "\nScore saved for trend tracking.")
	}

	return out.String(), nil
}

// evaluateRule checks whether a config violates the given rule.
// It returns a finding and true if the rule is violated, or zero-value and false otherwise.
func evaluateRule(rule db.RenovateRule, config map[string]interface{}) (renovateFinding, bool) {
	dontViolation := checkDontConfig(rule, config)
	missingViolation := checkMissingConfig(rule, config)

	if dontViolation || missingViolation {
		desc := buildViolationDescription(rule, config, dontViolation, missingViolation)
		return renovateFinding{
			RuleID:      rule.RuleID,
			Severity:    rule.Severity,
			Title:       rule.Title,
			Description: desc,
		}, true
	}
	return renovateFinding{}, false
}

// checkDontConfig parses the rule's DontConfig as JSON and checks if the user
// config contains matching top-level keys with similar structure.
func checkDontConfig(rule db.RenovateRule, config map[string]interface{}) bool {
	if rule.DontConfig == "" || rule.DontConfig == "{}" {
		return false
	}

	var dontMap map[string]interface{}
	if err := json.Unmarshal([]byte(rule.DontConfig), &dontMap); err != nil {
		return false
	}

	// Check if config contains the problematic pattern described in dont_config.
	for key, dontVal := range dontMap {
		configVal, exists := config[key]
		if !exists {
			continue
		}
		if matchesValue(configVal, dontVal) {
			return true
		}
	}
	return false
}

// checkMissingConfig checks if the rule's DoConfig references keys that are
// absent from the user config, indicating missing best-practice configuration.
// Only triggers when DontConfig is empty (i.e., the rule is about absence, not bad values).
func checkMissingConfig(rule db.RenovateRule, config map[string]interface{}) bool {
	if rule.DoConfig == "" || rule.DoConfig == "{}" {
		return false
	}

	// Rules with a non-empty DontConfig are checked via checkDontConfig.
	// Only check for missing config when the rule is purely about absence.
	if rule.DontConfig != "" && rule.DontConfig != "{}" {
		return false
	}

	var doMap map[string]interface{}
	if err := json.Unmarshal([]byte(rule.DoConfig), &doMap); err != nil {
		return false
	}

	// If the do_config recommends top-level keys that are absent, flag it.
	for key := range doMap {
		// Skip $comment keys — they are advisory, not structural.
		if key == "$comment" {
			continue
		}
		if _, exists := config[key]; !exists {
			return true
		}
	}
	return false
}

// matchesValue checks if configVal matches (or contains) dontVal.
// For simple types, it does direct comparison.
// For maps, it checks that the config map contains the same keys with matching values.
// For arrays, it checks if any array element matches.
func matchesValue(configVal, dontVal interface{}) bool {
	switch dv := dontVal.(type) {
	case bool:
		cv, ok := configVal.(bool)
		return ok && cv == dv
	case float64:
		cv, ok := configVal.(float64)
		return ok && cv == dv
	case string:
		cv, ok := configVal.(string)
		return ok && cv == dv
	case map[string]interface{}:
		cv, ok := configVal.(map[string]interface{})
		if !ok {
			return false
		}
		// All keys in dontVal must be present and match in configVal.
		for k, v := range dv {
			configSubVal, exists := cv[k]
			if !exists {
				return false
			}
			if !matchesValue(configSubVal, v) {
				return false
			}
		}
		return true
	case []interface{}:
		cv, ok := configVal.([]interface{})
		if !ok {
			return false
		}
		// Check if any element in the config array matches any element in dont array.
		for _, dontElem := range dv {
			for _, configElem := range cv {
				if matchesValue(configElem, dontElem) {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

// buildViolationDescription creates a human-readable description of why the rule was violated.
func buildViolationDescription(rule db.RenovateRule, config map[string]interface{}, isDont, isMissing bool) string {
	if isDont {
		return violationFromDontConfig(rule)
	}
	if isMissing {
		return violationFromMissingConfig(rule, config)
	}
	return rule.Description
}

// violationFromDontConfig describes what bad pattern was found.
func violationFromDontConfig(rule db.RenovateRule) string {
	var dontMap map[string]interface{}
	if err := json.Unmarshal([]byte(rule.DontConfig), &dontMap); err != nil {
		return rule.Description
	}
	keys := make([]string, 0, len(dontMap))
	for k := range dontMap {
		keys = append(keys, k)
	}
	return fmt.Sprintf("%s found in config", strings.Join(keys, ", "))
}

// violationFromMissingConfig describes what recommended keys are absent.
func violationFromMissingConfig(rule db.RenovateRule, config map[string]interface{}) string {
	var doMap map[string]interface{}
	if err := json.Unmarshal([]byte(rule.DoConfig), &doMap); err != nil {
		return rule.Description
	}
	var missing []string
	for k := range doMap {
		if k == "$comment" {
			continue
		}
		if _, exists := config[k]; !exists {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return rule.Description
	}
	return fmt.Sprintf("%s not configured", strings.Join(missing, ", "))
}
