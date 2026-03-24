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

const (
	defaultGoVulnURL = "https://vuln.go.dev"
	defaultNVDURL    = "https://services.nvd.nist.gov"
)

// FetchOptions controls behaviour of FetchVulns.
type FetchOptions struct {
	// GoModPath is the path to a go.mod file. Modules are parsed from it.
	// If empty AND Modules is empty, FetchVulns returns an error.
	GoModPath string
	// Modules overrides GoModPath when non-empty.
	Modules []string
	// NVDAPIKey enables NVD enrichment (CVSS scores). Leave empty to skip.
	NVDAPIKey string
	// GoVulnURL overrides the go-vuln base URL (for testing).
	GoVulnURL string
	// NVDURL overrides the NVD base URL (for testing).
	NVDURL string
	// Quiet suppresses progress output to the io.Writer.
	Quiet bool
	// Out receives progress messages. Defaults to io.Discard.
	Out io.Writer
	// NVDDelay is the pause between NVD API calls to respect rate limits.
	// Zero uses the default of 650ms.
	NVDDelay time.Duration
}

// FetchResult summarises the outcome of FetchVulns.
type FetchResult struct {
	ModulesChecked int
	CVEsFound      int
	CVEsEnriched   int // enriched via NVD CVSS
}

// osvEntry is the OSV JSON structure returned by GET /v1/vulns.
type osvEntry struct {
	ID       string   `json:"id"`
	Aliases  []string `json:"aliases"`
	Summary  string   `json:"summary"`
	Details  string   `json:"details"`
	Affected []struct {
		Package struct {
			Name string `json:"name"`
		} `json:"package"`
		Ranges []osvRange `json:"ranges"`
	} `json:"affected"`
}

// osvRange represents a version range in the OSV schema.
type osvRange struct {
	Type   string     `json:"type"`
	Events []osvEvent `json:"events"`
}

// osvEvent represents a single version event in an OSV range.
type osvEvent struct {
	Introduced string `json:"introduced,omitempty"`
	Fixed      string `json:"fixed,omitempty"`
}

// nvdCVE is a minimal subset of the NVD CVE 2.0 API response needed for CVSS enrichment.
type nvdCVE struct {
	Vulnerabilities []struct {
		CVE struct {
			ID      string `json:"id"`
			Metrics struct {
				V31 []struct {
					CVSSData struct {
						BaseScore    float64 `json:"baseScore"`
						BaseSeverity string  `json:"baseSeverity"`
					} `json:"cvssData"`
				} `json:"cvssMetricV31"`
				V30 []struct {
					CVSSData struct {
						BaseScore    float64 `json:"baseScore"`
						BaseSeverity string  `json:"baseSeverity"`
					} `json:"cvssData"`
				} `json:"cvssMetricV30"`
			} `json:"metrics"`
		} `json:"cve"`
	} `json:"vulnerabilities"`
}

// FetchVulns fetches Go vulnerability data for the given modules and stores
// the results in the vuln_cache table.
//
// Strategy:
//  1. GET https://vuln.go.dev/v1/vulns -- one HTTP call, returns all Go advisories.
//  2. Filter in-memory by the module set from go.mod.
//  3. If NVDAPIKey is set, enrich each found CVE with full CVSS scores from NVD.
func FetchVulns(ctx context.Context, store *db.Store, opts FetchOptions) (FetchResult, error) {
	var result FetchResult
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	logf := func(format string, args ...any) {
		if !opts.Quiet {
			fmt.Fprintf(out, format+"\n", args...)
		}
	}

	// -- Build module set ---------------------------------------------------
	modules := opts.Modules
	if modules == nil {
		if opts.GoModPath == "" {
			return result, fmt.Errorf("either GoModPath or Modules must be provided")
		}
		var err error
		modules, err = ParseGoMod(opts.GoModPath)
		if err != nil {
			return result, fmt.Errorf("parse go.mod: %w", err)
		}
	}
	if len(modules) == 0 {
		return result, nil
	}
	result.ModulesChecked = len(modules)
	modSet := make(map[string]bool, len(modules))
	for _, m := range modules {
		modSet[m] = true
	}
	logf("Fetching Go vulnerability data for %d modules...", len(modules))

	// -- Fetch all Go vulns (one HTTP call) ---------------------------------
	goVulnBase := opts.GoVulnURL
	if goVulnBase == "" {
		goVulnBase = defaultGoVulnURL
	}
	entries, err := fetchAllGoVulns(ctx, goVulnBase)
	if err != nil {
		return result, fmt.Errorf("fetch go-vuln database: %w", err)
	}
	logf("  Fetched %d entries from go-vuln database", len(entries))

	// -- Filter and store ---------------------------------------------------
	nvdBase := opts.NVDURL
	if nvdBase == "" {
		nvdBase = defaultNVDURL
	}

	for _, entry := range entries {
		for _, affected := range entry.Affected {
			module := affected.Package.Name
			if !modSet[module] {
				continue
			}

			// Extract CVE IDs from aliases (prefer CVE- prefix).
			cveID := entry.ID // fallback to GO- ID
			for _, alias := range entry.Aliases {
				if strings.HasPrefix(alias, "CVE-") {
					cveID = alias
					break
				}
			}

			// Extract affected/fixed version from SEMVER ranges.
			affectedVer, fixedVer := extractVersionRange(affected.Ranges)

			// Severity: go-vuln does not always provide one; default to UNKNOWN.
			// NVD enrichment will override this when an API key is provided.
			severity := "UNKNOWN"

			description := entry.Summary
			if description == "" {
				description = entry.Details
			}
			if len(description) > 500 {
				description = description[:500] + "..."
			}

			if err := store.UpsertVulnCache(module, cveID, severity, affectedVer, fixedVer, description); err != nil {
				return result, fmt.Errorf("upsert vuln %s/%s: %w", module, cveID, err)
			}
			result.CVEsFound++

			// -- NVD enrichment -------------------------------------------------
			if opts.NVDAPIKey != "" && strings.HasPrefix(cveID, "CVE-") {
				score, nvdSeverity, enrichDesc, err := fetchNVDCVSS(ctx, nvdBase, cveID, opts.NVDAPIKey)
				if err != nil {
					// Non-fatal: log and continue.
					logf("  WARN: NVD enrichment failed for %s: %v", cveID, err)
					continue
				}
				if nvdSeverity != "" {
					enrichedDesc := fmt.Sprintf("[CVSS:%.1f %s] %s", score, nvdSeverity, enrichDesc)
					if len(enrichedDesc) > 500 {
						enrichedDesc = enrichedDesc[:500] + "..."
					}
					if err := store.UpsertVulnCache(module, cveID, nvdSeverity, affectedVer, fixedVer, enrichedDesc); err != nil {
						return result, fmt.Errorf("upsert enriched vuln %s/%s: %w", module, cveID, err)
					}
					result.CVEsEnriched++
				}
				// Respect NVD rate limit: 50 req/30s with key -> ~600ms between calls is safe.
				nvdDelay := opts.NVDDelay
				if nvdDelay == 0 {
					nvdDelay = 650 * time.Millisecond
				}
				if nvdDelay > 0 {
					select {
					case <-ctx.Done():
						return result, ctx.Err()
					case <-time.After(nvdDelay):
					}
				}
			}
		}
	}

	logf("Done. CVEs found: %d, enriched via NVD: %d", result.CVEsFound, result.CVEsEnriched)
	return result, nil
}

// fetchAllGoVulns calls GET /v1/vulns and returns all OSV entries.
func fetchAllGoVulns(ctx context.Context, baseURL string) ([]osvEntry, error) {
	if err := requireHTTPS(baseURL); err != nil {
		return nil, fmt.Errorf("go-vuln URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/vulns", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := secureHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("go-vuln API returned %d", resp.StatusCode)
	}

	var entries []osvEntry
	if err := json.NewDecoder(limitedBody(resp.Body)).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode go-vuln response: %w", err)
	}
	return entries, nil
}

// fetchNVDCVSS retrieves the CVSS base score and severity for a CVE from NVD.
func fetchNVDCVSS(ctx context.Context, baseURL, cveID, apiKey string) (score float64, severity, description string, err error) {
	if err := requireHTTPS(baseURL); err != nil {
		return 0, "", "", fmt.Errorf("NVD URL: %w", err)
	}
	url := fmt.Sprintf("%s/rest/json/cve/2.0/%s", baseURL, cveID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", "", err
	}
	req.Header.Set("apiKey", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := secureHTTPClient.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, "", "", nil // CVE not in NVD -- not an error
	}
	if resp.StatusCode != http.StatusOK {
		return 0, "", "", fmt.Errorf("NVD returned %d for %s", resp.StatusCode, cveID)
	}

	var nvd nvdCVE
	if err := json.NewDecoder(limitedBody(resp.Body)).Decode(&nvd); err != nil {
		return 0, "", "", fmt.Errorf("decode NVD response: %w", err)
	}
	if len(nvd.Vulnerabilities) == 0 {
		return 0, "", "", nil
	}

	cve := nvd.Vulnerabilities[0].CVE
	_ = cve.ID // accessed for clarity; severity is the goal

	// Prefer CVSSv3.1, fall back to v3.0.
	if len(cve.Metrics.V31) > 0 {
		d := cve.Metrics.V31[0].CVSSData
		return d.BaseScore, d.BaseSeverity, description, nil
	}
	if len(cve.Metrics.V30) > 0 {
		d := cve.Metrics.V30[0].CVSSData
		return d.BaseScore, d.BaseSeverity, description, nil
	}
	return 0, "", "", nil
}

// extractVersionRange returns (affectedVersionRange, fixedVersion) from OSV SEMVER ranges.
func extractVersionRange(ranges []osvRange) (affected, fixed string) {
	for _, r := range ranges {
		if r.Type != "SEMVER" {
			continue
		}
		var introduced, fixedVer string
		for _, ev := range r.Events {
			if ev.Introduced != "" && ev.Introduced != "0" {
				introduced = ">= " + ev.Introduced
			} else if ev.Introduced == "0" {
				introduced = ">= 0"
			}
			if ev.Fixed != "" {
				fixedVer = ev.Fixed
			}
		}
		if introduced != "" || fixedVer != "" {
			return introduced, fixedVer
		}
	}
	return "", ""
}
