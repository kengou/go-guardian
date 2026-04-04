// Package tools implements the MCP tool handlers for go-guardian.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// deprecatedOptions maps deprecated Renovate option names to their recommended replacements.
var deprecatedOptions = map[string]string{
	"ignoreUnstable":        "use 'ignoreUnstable' is deprecated — review Renovate docs for current equivalent",
	"separatePatchReleases": "use 'separateMinorPatch' instead",
	"versionScheme":         "use 'versioning' instead",
	"unpublishSafe":         "use 'extends: [\"npm:unpublishSafe\"]' instead",
}

// RegisterValidateRenovateConfig registers the validate_renovate_config tool on the MCP server.
func RegisterValidateRenovateConfig(s ToolRegistrar, store *db.Store) {
	tool := mcp.NewTool("validate_renovate_config",
		mcp.WithDescription("Validate a Renovate JSON configuration file. Checks syntax, deprecated options, structure, and optionally runs a dry-run if RENOVATE_TOKEN is set."),
		mcp.WithString("config_path", mcp.Required(), mcp.Description("Path to the Renovate JSON configuration file to validate")),
	)
	s.AddTool(tool, handleValidateRenovateConfig(store))
}

func handleValidateRenovateConfig(store *db.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		configPath, err := request.RequireString("config_path")
		if err != nil {
			return mcp.NewToolResultError("config_path is required"), nil
		}

		var out strings.Builder
		var warnings, errs int

		fmt.Fprintf(&out, "=== Renovate Config Validation: %s ===\n\n", configPath)
		fmt.Fprintln(&out, "Pass 1: Local Validation")

		// Read the file.
		data, err := os.ReadFile(configPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot read config file: %v", err)), nil
		}

		// Parse JSON.
		var raw any
		if err := json.Unmarshal(data, &raw); err != nil {
			var syntaxErr *json.SyntaxError
			if errors.As(err, &syntaxErr) {
				line, col := positionFromOffset(data, syntaxErr.Offset)
				fmt.Fprintf(&out, "✗ ERR: Invalid JSON syntax at line %d, column %d: %s\n", line, col, syntaxErr.Error())
			} else {
				fmt.Fprintf(&out, "✗ ERR: Invalid JSON: %s\n", err.Error())
			}
			errs++
			fmt.Fprintf(&out, "\nSummary: %d warning(s), %d error(s)\n", warnings, errs)
			return mcp.NewToolResultText(out.String()), nil
		}
		fmt.Fprintln(&out, "✓ Valid JSON syntax")

		// Top-level must be a JSON object.
		rootMap, ok := raw.(map[string]any)
		if !ok {
			fmt.Fprintln(&out, "✗ ERR: Top-level value must be a JSON object")
			errs++
			fmt.Fprintf(&out, "\nSummary: %d warning(s), %d error(s)\n", warnings, errs)
			return mcp.NewToolResultText(out.String()), nil
		}

		// Check deprecated options.
		w := checkDeprecatedOptions(rootMap, &out)
		warnings += w

		// Validate structure.
		e, w2 := validateStructure(rootMap, &out)
		errs += e
		warnings += w2

		if errs == 0 && warnings == 0 {
			fmt.Fprintln(&out, "✓ Structure valid")
		} else if errs == 0 {
			fmt.Fprintln(&out, "✓ Structure valid (with warnings)")
		}

		// Pass 2: Dry-run.
		fmt.Fprintln(&out, "\nPass 2: Dry Run")
		dw, de := runDryRun(configPath, &out)
		warnings += dw
		errs += de

		fmt.Fprintf(&out, "\nSummary: %d warning(s), %d error(s)\n", warnings, errs)
		return mcp.NewToolResultText(out.String()), nil
	}
}

// positionFromOffset converts a byte offset into a 1-based line and column.
func positionFromOffset(data []byte, offset int64) (line, col int) {
	line = 1
	col = 1
	for i := int64(0); i < offset && i < int64(len(data)); i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// checkDeprecatedOptions scans top-level keys and packageRules entries for deprecated options.
func checkDeprecatedOptions(root map[string]any, out *strings.Builder) (warnings int) {
	// Check top-level keys.
	for key, hint := range deprecatedOptions {
		if _, found := root[key]; found {
			fmt.Fprintf(out, "⚠ WARN: Deprecated option '%s' found — %s\n", key, hint)
			warnings++
		}
	}

	// Check automergeType: "branch-merge-commit" at top-level.
	if v, ok := root["automergeType"].(string); ok && v == "branch-merge-commit" {
		fmt.Fprintln(out, "⚠ WARN: Deprecated automergeType value 'branch-merge-commit' — use 'branch' or 'pr' instead")
		warnings++
	}

	// Check inside packageRules entries.
	if rules, ok := root["packageRules"].([]any); ok {
		for i, entry := range rules {
			ruleMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			for key, hint := range deprecatedOptions {
				if _, found := ruleMap[key]; found {
					fmt.Fprintf(out, "⚠ WARN: Deprecated option '%s' found in packageRules[%d] — %s\n", key, i, hint)
					warnings++
				}
			}
			if v, ok := ruleMap["automergeType"].(string); ok && v == "branch-merge-commit" {
				fmt.Fprintf(out, "⚠ WARN: Deprecated automergeType value 'branch-merge-commit' in packageRules[%d] — use 'branch' or 'pr' instead\n", i)
				warnings++
			}
		}
	}

	return warnings
}

// validateStructure checks extends and packageRules structure.
func validateStructure(root map[string]any, out *strings.Builder) (errs, warnings int) {
	// Validate extends.
	if ext, exists := root["extends"]; exists {
		arr, ok := ext.([]any)
		if !ok {
			fmt.Fprintln(out, "✗ ERR: 'extends' must be an array of strings")
			errs++
		} else if len(arr) == 0 {
			fmt.Fprintln(out, "⚠ WARN: 'extends' array is empty — consider adding a base preset (e.g., \"config:recommended\")")
			warnings++
		} else {
			for i, v := range arr {
				if _, ok := v.(string); !ok {
					fmt.Fprintf(out, "✗ ERR: 'extends[%d]' must be a string, got %T\n", i, v)
					errs++
				}
			}
		}
	}

	// Validate packageRules.
	if pr, exists := root["packageRules"]; exists {
		arr, ok := pr.([]any)
		if !ok {
			fmt.Fprintln(out, "✗ ERR: 'packageRules' must be an array of objects")
			errs++
		} else {
			for i, v := range arr {
				if _, ok := v.(map[string]any); !ok {
					fmt.Fprintf(out, "✗ ERR: 'packageRules[%d]' must be an object, got %T\n", i, v)
					errs++
				}
			}
		}
	}

	return errs, warnings
}

// runDryRun attempts a renovate --dry-run if RENOVATE_TOKEN and the CLI are available.
func runDryRun(configPath string, out *strings.Builder) (warnings, errs int) {
	token := os.Getenv("RENOVATE_TOKEN")
	if token == "" {
		fmt.Fprintln(out, "⚠ Skipped: RENOVATE_TOKEN not set")
		fmt.Fprintln(out, "  To enable: export RENOVATE_TOKEN=your-github-token")
		return 0, 0
	}

	renovateBin, err := exec.LookPath("renovate")
	if err != nil {
		fmt.Fprintln(out, "⚠ Skipped: 'renovate' CLI not found in PATH")
		fmt.Fprintln(out, "  Install: npm install -g renovate")
		return 0, 0
	}

	cmd := exec.Command(renovateBin, "--dry-run", "--config-file", configPath)
	cmd.Env = append(os.Environ(), "LOG_LEVEL=warn")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(out, "⚠ WARN: dry-run exited with error: %v\n", err)
		warnings++
	}

	combined := stderr.String() + stdout.String()
	if combined != "" {
		lines := strings.Split(strings.TrimSpace(combined), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			lower := strings.ToLower(line)
			if strings.Contains(lower, "error") {
				fmt.Fprintf(out, "✗ ERR: %s\n", line)
				errs++
			} else if strings.Contains(lower, "warn") {
				fmt.Fprintf(out, "⚠ WARN: %s\n", line)
				warnings++
			} else {
				fmt.Fprintf(out, "  %s\n", line)
			}
		}
	} else {
		fmt.Fprintln(out, "✓ Dry run passed with no warnings")
	}

	return warnings, errs
}
