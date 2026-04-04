package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kengou/go-guardian/mcp-server/db"
)

const (
	defaultGoVulnURL = "https://vuln.go.dev"
	defaultNVDURL    = "https://services.nvd.nist.gov"
)

// PrefetchStatus tracks the current state of a vulnerability prefetch operation.
type PrefetchStatus struct {
	mu          sync.RWMutex
	Phase       string    `json:"phase"`        // "idle", "go-vuln", "nvd", "done", "error"
	Source      string    `json:"source"`        // current source being fetched
	Progress    int       `json:"progress"`      // CVEs processed so far
	Total       int       `json:"total"`         // total CVEs to process
	CVEsFound   int       `json:"cves_found"`    // total CVEs discovered
	CVEsEnriched int      `json:"cves_enriched"` // enriched via NVD
	LastRefresh time.Time `json:"last_refresh"`  // when the last fetch completed
	Error       string    `json:"error"`         // last error message, if any
}

// Snapshot returns a copy of the current status safe for JSON marshalling.
func (ps *PrefetchStatus) Snapshot() PrefetchStatus {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return PrefetchStatus{
		Phase:        ps.Phase,
		Source:       ps.Source,
		Progress:     ps.Progress,
		Total:        ps.Total,
		CVEsFound:    ps.CVEsFound,
		CVEsEnriched: ps.CVEsEnriched,
		LastRefresh:  ps.LastRefresh,
		Error:        ps.Error,
	}
}

// SetPhase updates the current fetch phase and source label.
func (ps *PrefetchStatus) SetPhase(phase, source string) {
	ps.mu.Lock()
	ps.Phase = phase
	ps.Source = source
	ps.mu.Unlock()
}

// SetProgress updates the CVE processing progress.
func (ps *PrefetchStatus) SetProgress(progress, total int) {
	ps.mu.Lock()
	ps.Progress = progress
	ps.Total = total
	ps.mu.Unlock()
}

// SetDone marks the prefetch as complete.
func (ps *PrefetchStatus) SetDone(found, enriched int) {
	ps.mu.Lock()
	ps.Phase = "done"
	ps.Source = ""
	ps.CVEsFound = found
	ps.CVEsEnriched = enriched
	ps.LastRefresh = time.Now()
	ps.Error = ""
	ps.mu.Unlock()
}

// SetError marks the prefetch as failed.
func (ps *PrefetchStatus) SetError(err string) {
	ps.mu.Lock()
	ps.Phase = "error"
	ps.Error = err
	ps.mu.Unlock()
}

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
	// Status receives live progress updates. May be nil.
	Status *PrefetchStatus
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
//  1. GET https://vuln.go.dev/ID/index.json -- list of all advisory IDs.
//  2. GET https://vuln.go.dev/ID/{id}.json for each -- individual OSV entries.
//  3. Filter in-memory by the module set from go.mod.
//  4. If NVDAPIKey is set, enrich each found CVE with full CVSS scores from NVD.
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
	status := opts.Status // may be nil

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

	// -- Fetch matching Go vulns -------------------------------------------
	goVulnBase := opts.GoVulnURL
	if goVulnBase == "" {
		goVulnBase = defaultGoVulnURL
	}
	if status != nil {
		status.SetPhase("go-vuln", "vuln.go.dev")
	}
	entries, err := fetchAllGoVulns(ctx, goVulnBase, modSet)
	if err != nil {
		if status != nil {
			status.SetError(err.Error())
		}
		return result, fmt.Errorf("fetch go-vuln database: %w", err)
	}
	logf("  Fetched %d matching advisories from go-vuln database", len(entries))

	// -- Filter and store ---------------------------------------------------
	nvdBase := opts.NVDURL
	if nvdBase == "" {
		nvdBase = defaultNVDURL
	}

	// Count total CVEs for progress tracking.
	totalCVEs := 0
	for _, entry := range entries {
		for _, affected := range entry.Affected {
			if modSet[affected.Package.Name] {
				totalCVEs++
			}
		}
	}
	processedCVEs := 0

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

			if err := store.UpsertVulnCache(module, cveID, severity, affectedVer, fixedVer, description, "go-vuln"); err != nil {
				return result, fmt.Errorf("upsert vuln %s/%s: %w", module, cveID, err)
			}
			result.CVEsFound++
			processedCVEs++

			if status != nil {
				status.SetProgress(processedCVEs, totalCVEs)
			}

			// -- NVD enrichment -------------------------------------------------
			if opts.NVDAPIKey != "" && strings.HasPrefix(cveID, "CVE-") {
				if status != nil {
					status.SetPhase("nvd", "NVD ("+cveID+")")
				}
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
					if err := store.UpsertVulnCache(module, cveID, nvdSeverity, affectedVer, fixedVer, enrichedDesc, "nvd"); err != nil {
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

	if status != nil {
		status.SetDone(result.CVEsFound, result.CVEsEnriched)
	}
	logf("Done. CVEs found: %d, enriched via NVD: %d", result.CVEsFound, result.CVEsEnriched)
	return result, nil
}

// moduleIndexEntry represents one entry in the vuln.go.dev /index/modules.json response.
type moduleIndexEntry struct {
	Path  string `json:"path"`
	Vulns []struct {
		ID       string `json:"id"`
		Modified string `json:"modified"`
		Fixed    string `json:"fixed,omitempty"`
	} `json:"vulns"`
}

// fetchAllGoVulns fetches Go vulnerability data relevant to the given module set.
// It uses the two-step API:
//  1. GET /index/modules.json — module-to-advisory mapping (one HTTP call).
//  2. GET /ID/{id}.json — full advisory for each matching advisory.
//
// This is much more efficient than fetching all advisories, since only advisories
// affecting the user's modules are downloaded.
func fetchAllGoVulns(ctx context.Context, baseURL string, modSet map[string]bool) ([]osvEntry, error) {
	if err := requireHTTPS(baseURL); err != nil {
		return nil, fmt.Errorf("go-vuln URL: %w", err)
	}

	// Step 1: fetch the module index.
	moduleIndex, err := fetchGoVulnModuleIndex(ctx, baseURL)
	if err != nil {
		return nil, err
	}

	// Step 2: collect advisory IDs that match the user's modules.
	idSet := make(map[string]bool)
	for _, m := range moduleIndex {
		if !modSet[m.Path] {
			continue
		}
		for _, v := range m.Vulns {
			idSet[v.ID] = true
		}
	}

	if len(idSet) == 0 {
		return nil, nil
	}

	// Step 3: fetch each matching advisory by ID.
	entries := make([]osvEntry, 0, len(idSet))
	for id := range idSet {
		entry, err := fetchGoVulnEntry(ctx, baseURL, id)
		if err != nil {
			continue // skip entries that fail to fetch
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// fetchGoVulnModuleIndex fetches GET /index/modules.json.
func fetchGoVulnModuleIndex(ctx context.Context, baseURL string) ([]moduleIndexEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/index/modules.json", nil)
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
		return nil, fmt.Errorf("go-vuln modules index returned %d", resp.StatusCode)
	}

	var index []moduleIndexEntry
	if err := json.NewDecoder(limitedBody(resp.Body)).Decode(&index); err != nil {
		return nil, fmt.Errorf("decode go-vuln modules index: %w", err)
	}
	return index, nil
}

// fetchGoVulnEntry fetches GET /ID/{id}.json and returns a single OSV entry.
func fetchGoVulnEntry(ctx context.Context, baseURL, id string) (osvEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/ID/"+id+".json", nil)
	if err != nil {
		return osvEntry{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := secureHTTPClient.Do(req)
	if err != nil {
		return osvEntry{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return osvEntry{}, fmt.Errorf("go-vuln entry %s returned %d", id, resp.StatusCode)
	}

	var entry osvEntry
	if err := json.NewDecoder(limitedBody(resp.Body)).Decode(&entry); err != nil {
		return osvEntry{}, fmt.Errorf("decode go-vuln entry %s: %w", id, err)
	}
	return entry, nil
}

// fetchNVDCVSS retrieves the CVSS base score and severity for a CVE from NVD.
func fetchNVDCVSS(ctx context.Context, baseURL, cveID, apiKey string) (score float64, severity, description string, err error) {
	if err := requireHTTPS(baseURL); err != nil {
		return 0, "", "", fmt.Errorf("NVD URL: %w", err)
	}
	url := fmt.Sprintf("%s/rest/json/cves/2.0?cveId=%s", baseURL, cveID)
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
