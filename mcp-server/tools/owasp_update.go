package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kengou/go-guardian/mcp-server/db"
)

const defaultGHSAURL = "https://api.github.com"

// cweToOWASP maps CWE identifiers to OWASP Top 10 2021 categories.
// Only CWEs commonly seen in Go advisories are listed.
var cweToOWASP = map[string]string{
	// A01: Broken Access Control
	"CWE-22":  "A01-BrokenAccessControl",
	"CWE-23":  "A01-BrokenAccessControl",
	"CWE-284": "A01-BrokenAccessControl",
	"CWE-285": "A01-BrokenAccessControl",
	"CWE-639": "A01-BrokenAccessControl",
	"CWE-862": "A01-BrokenAccessControl",
	// A02: Cryptographic Failures
	"CWE-310": "A02-CryptographicFailures",
	"CWE-311": "A02-CryptographicFailures",
	"CWE-326": "A02-CryptographicFailures",
	"CWE-327": "A02-CryptographicFailures",
	"CWE-328": "A02-CryptographicFailures",
	"CWE-330": "A02-CryptographicFailures",
	"CWE-347": "A02-CryptographicFailures",
	// A03: Injection
	"CWE-74":  "A03-Injection",
	"CWE-77":  "A03-Injection",
	"CWE-78":  "A03-Injection",
	"CWE-79":  "A03-Injection",
	"CWE-89":  "A03-Injection",
	"CWE-90":  "A03-Injection",
	"CWE-94":  "A03-Injection",
	"CWE-643": "A03-Injection",
	// A04: Insecure Design
	"CWE-209": "A04-InsecureDesign",
	"CWE-400": "A04-InsecureDesign",
	"CWE-770": "A04-InsecureDesign",
	// A05: Security Misconfiguration
	"CWE-16":  "A05-SecurityMisconfiguration",
	"CWE-732": "A05-SecurityMisconfiguration",
	// A06: Vulnerable and Outdated Components (fed by vuln scanner)
	// A07: Identification and Authentication Failures
	"CWE-287": "A07-AuthFailures",
	"CWE-306": "A07-AuthFailures",
	"CWE-384": "A07-AuthFailures",
	"CWE-798": "A07-AuthFailures",
	// A08: Software and Data Integrity Failures
	"CWE-345": "A08-IntegrityFailures",
	"CWE-502": "A08-IntegrityFailures",
	// A09: Security Logging and Monitoring Failures
	"CWE-117": "A09-LoggingFailures",
	"CWE-532": "A09-LoggingFailures",
	// A10: Server-Side Request Forgery
	"CWE-918": "A10-SSRF",
}

// OWASPUpdateOptions controls the GHSA advisory fetch.
type OWASPUpdateOptions struct {
	// GHSAURL overrides the GitHub API base URL (for testing).
	GHSAURL string
	// GitHubToken is optional; increases rate limits from 60/h to 5000/h.
	GitHubToken string
	// Quiet suppresses progress output.
	Quiet bool
	// Out receives progress messages. Defaults to io.Discard.
	Out io.Writer
	// Since only fetches advisories updated after this time. Zero = fetch all.
	Since time.Time
	// PageDelay is the pause between paginated API calls to respect rate limits.
	// Zero uses the default of 200ms. Set to a negative value or a specific
	// duration in tests to speed up execution.
	PageDelay time.Duration
}

// OWASPUpdateResult summarises what was stored.
type OWASPUpdateResult struct {
	AdvisoriesFetched int
	PatternsStored    int
	CategoriesUpdated map[string]int // OWASP category -> count
}

// ghsaAdvisory is a minimal subset of the GHSA advisory API response.
type ghsaAdvisory struct {
	GHSAID    string `json:"ghsa_id"`
	CVEID     string `json:"cve_id"`
	Summary   string `json:"summary"`
	Severity  string `json:"severity"`
	UpdatedAt string `json:"updated_at"`
	CWEs      []struct {
		CWEID string `json:"cwe_id"`
		Name  string `json:"name"`
	} `json:"cwes"`
	Vulnerabilities []struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
		VulnerableVersionRange string `json:"vulnerable_version_range"`
		FirstPatchedVersion    *struct {
			Identifier string `json:"identifier"`
		} `json:"first_patched_version"`
	} `json:"vulnerabilities"`
}

// UpdateOWASPRules fetches Go advisories from GHSA, maps CWEs to OWASP
// categories, and stores new findings in owasp_findings and anti_patterns.
func UpdateOWASPRules(ctx context.Context, store *db.Store, opts OWASPUpdateOptions) (OWASPUpdateResult, error) {
	result := OWASPUpdateResult{
		CategoriesUpdated: make(map[string]int),
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	logf := func(format string, args ...any) {
		if !opts.Quiet {
			fmt.Fprintf(out, format+"\n", args...)
		}
	}

	base := opts.GHSAURL
	if base == "" {
		base = defaultGHSAURL
	}

	logf("Fetching Go advisories from GitHub Security Advisory database...")

	pageDelay := opts.PageDelay
	if pageDelay == 0 {
		pageDelay = 200 * time.Millisecond
	}

	advisories, err := fetchGHSAAdvisories(ctx, base, opts.GitHubToken, opts.Since, pageDelay)
	if err != nil {
		return result, fmt.Errorf("fetch GHSA advisories: %w", err)
	}
	result.AdvisoriesFetched = len(advisories)
	logf("  Fetched %d Go advisories", len(advisories))

	for _, adv := range advisories {
		// Map CWEs to OWASP categories; skip advisories with no known mapping.
		categories := cwesToOWASPCategories(adv.CWEs)
		if len(categories) == 0 {
			continue
		}

		// Build a file pattern from affected Go packages.
		filePattern := goFilePatternFromAdvisory(adv)

		// Build a finding description.
		finding := adv.Summary
		if finding == "" {
			finding = adv.GHSAID
		}
		id := adv.CVEID
		if id == "" {
			id = adv.GHSAID
		}

		// Build a fix hint from the first patched version, if available.
		fix := fixHintFromAdvisory(adv)

		for _, category := range categories {
			if err := store.InsertOWASPFinding(category, filePattern, finding, fix); err != nil {
				return result, fmt.Errorf("insert owasp finding %s: %w", id, err)
			}
			result.CategoriesUpdated[category]++
			result.PatternsStored++
		}

		// Also store as an anti-pattern so query_knowledge can surface it.
		for _, category := range categories {
			patternID := fmt.Sprintf("GHSA-%s-%s", strings.ReplaceAll(category, "-", ""), id)
			if len(patternID) > 60 {
				patternID = patternID[:60]
			}
			dontCode := fmt.Sprintf("// Affected: %s\n// %s", filePattern, finding)
			doCode := fix
			// InsertAntiPattern signature: (patternID, description, dontCode, doCode, source, category)
			if err := store.InsertAntiPattern(patternID, finding, dontCode, doCode, "ghsa", category); err != nil {
				// Non-fatal: pattern may already exist (INSERT OR IGNORE).
				continue
			}
		}
	}

	// Record the scan so check_staleness tracks freshness.
	if err := store.UpdateScanHistory("owasp_rules", "global", result.PatternsStored); err != nil {
		return result, fmt.Errorf("update scan history: %w", err)
	}

	logf("Done. Patterns stored: %d across %d OWASP categories",
		result.PatternsStored, len(result.CategoriesUpdated))
	return result, nil
}

// fetchGHSAAdvisories pages through the GHSA advisory API for Go ecosystem advisories.
func fetchGHSAAdvisories(ctx context.Context, base, token string, since time.Time, pageDelay time.Duration) ([]ghsaAdvisory, error) {
	if err := requireHTTPS(base); err != nil {
		return nil, fmt.Errorf("GHSA URL: %w", err)
	}
	var all []ghsaAdvisory
	page := 1
	for {
		url := fmt.Sprintf("%s/advisories?type=reviewed&ecosystem=go&per_page=100&page=%d", base, page)
		if !since.IsZero() {
			url += "&updated_after=" + since.UTC().Format(time.RFC3339)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := secureHTTPClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
			resp.Body.Close()
			break
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GHSA API returned %d on page %d", resp.StatusCode, page)
		}

		var pageAdvisories []ghsaAdvisory
		decodeErr := json.NewDecoder(limitedBody(resp.Body)).Decode(&pageAdvisories)
		io.Copy(io.Discard, resp.Body) //nolint:errcheck // drain so connection can be reused
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("decode page %d: %w", page, decodeErr)
		}
		if len(pageAdvisories) == 0 {
			break
		}
		all = append(all, pageAdvisories...)
		page++

		// Respect GitHub API rate limit: brief pause between pages.
		if pageDelay > 0 {
			select {
			case <-ctx.Done():
				return all, ctx.Err()
			case <-time.After(pageDelay):
			}
		}
	}
	return all, nil
}

// cwesToOWASPCategories maps a slice of CWE structs to distinct OWASP categories.
func cwesToOWASPCategories(cwes []struct {
	CWEID string `json:"cwe_id"`
	Name  string `json:"name"`
}) []string {
	seen := make(map[string]bool)
	var categories []string
	for _, cwe := range cwes {
		if cat, ok := cweToOWASP[cwe.CWEID]; ok {
			if !seen[cat] {
				seen[cat] = true
				categories = append(categories, cat)
			}
		}
	}
	return categories
}

// goFilePatternFromAdvisory extracts a file pattern from affected package names.
func goFilePatternFromAdvisory(adv ghsaAdvisory) string {
	for _, v := range adv.Vulnerabilities {
		if strings.EqualFold(v.Package.Ecosystem, "go") && v.Package.Name != "" {
			return v.Package.Name
		}
	}
	return "*.go"
}

// fixHintFromAdvisory builds a short fix suggestion from the advisory data.
func fixHintFromAdvisory(adv ghsaAdvisory) string {
	for _, v := range adv.Vulnerabilities {
		if v.FirstPatchedVersion != nil && v.FirstPatchedVersion.Identifier != "" {
			return fmt.Sprintf("Upgrade to %s %s or later",
				v.Package.Name, v.FirstPatchedVersion.Identifier)
		}
	}
	if adv.CVEID != "" {
		return fmt.Sprintf("See %s for remediation guidance", adv.CVEID)
	}
	return fmt.Sprintf("See %s for remediation guidance", adv.GHSAID)
}
