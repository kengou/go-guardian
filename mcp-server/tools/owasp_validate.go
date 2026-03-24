package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateScanPath ensures scanPath is within projectRoot.
// Both paths are resolved to absolute form before comparison.
// This prevents path traversal via "../" sequences.
//
// Exact match (scanPath == projectRoot) is allowed so that agents can
// scan the root directory itself.
func validateScanPath(scanPath, projectRoot string) error {
	absPath, err := filepath.Abs(scanPath)
	if err != nil {
		return fmt.Errorf("invalid scan path: %w", err)
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("invalid project root: %w", err)
	}
	// Allow exact match or child paths only.
	if absPath != absRoot && !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) {
		return fmt.Errorf("scan path %q is outside project root %q", absPath, absRoot)
	}
	return nil
}
