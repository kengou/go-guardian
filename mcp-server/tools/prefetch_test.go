package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// makeGoVulnResponse builds a JSON array of osvEntry for the mock go-vuln server.
func makeGoVulnResponse(entries []osvEntry) []byte {
	data, _ := json.Marshal(entries)
	return data
}

// makeNVDResponse builds a minimal NVD CVE 2.0 API JSON response.
func makeNVDResponse(cveID string, baseScore float64, severity string) []byte {
	resp := map[string]any{
		"vulnerabilities": []map[string]any{
			{
				"cve": map[string]any{
					"id": cveID,
					"metrics": map[string]any{
						"cvssMetricV31": []map[string]any{
							{
								"cvssData": map[string]any{
									"baseScore":    baseScore,
									"baseSeverity": severity,
								},
							},
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(resp)
	return data
}

// testGoVulnEntries returns two OSV entries: one matching "github.com/example/vuln"
// and one matching "github.com/other/pkg" (which tests may or may not include in their module list).
func testGoVulnEntries() []osvEntry {
	return []osvEntry{
		{
			ID:      "GO-2024-0001",
			Aliases: []string{"CVE-2024-11111"},
			Summary: "SQL injection in example/vuln",
			Affected: []struct {
				Package struct {
					Name string `json:"name"`
				} `json:"package"`
				Ranges []osvRange `json:"ranges"`
			}{
				{
					Package: struct {
						Name string `json:"name"`
					}{Name: "github.com/example/vuln"},
					Ranges: []osvRange{
						{
							Type: "SEMVER",
							Events: []osvEvent{
								{Introduced: "0"},
								{Fixed: "1.2.3"},
							},
						},
					},
				},
			},
		},
		{
			ID:      "GO-2024-0002",
			Aliases: []string{"CVE-2024-22222", "GHSA-xxxx-yyyy"},
			Summary: "Path traversal in example/vuln",
			Affected: []struct {
				Package struct {
					Name string `json:"name"`
				} `json:"package"`
				Ranges []osvRange `json:"ranges"`
			}{
				{
					Package: struct {
						Name string `json:"name"`
					}{Name: "github.com/example/vuln"},
					Ranges: []osvRange{
						{
							Type: "SEMVER",
							Events: []osvEvent{
								{Introduced: "1.0.0"},
								{Fixed: "1.3.0"},
							},
						},
					},
				},
			},
		},
		{
			ID:      "GO-2024-0003",
			Aliases: []string{"CVE-2024-33333"},
			Summary: "XSS in other/pkg",
			Affected: []struct {
				Package struct {
					Name string `json:"name"`
				} `json:"package"`
				Ranges []osvRange `json:"ranges"`
			}{
				{
					Package: struct {
						Name string `json:"name"`
					}{Name: "github.com/other/pkg"},
					Ranges: []osvRange{
						{
							Type: "SEMVER",
							Events: []osvEvent{
								{Introduced: "0"},
							},
						},
					},
				},
			},
		},
	}
}

func TestFetchVulns_GoVulnOnly(t *testing.T) {
	// Mock go-vuln server returning 3 entries (2 match "example/vuln", 1 matches "other/pkg").
	goVulnSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/vulns" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(makeGoVulnResponse(testGoVulnEntries()))
	}))
	defer goVulnSrv.Close()

	store := newTestStore(t)
	result, err := FetchVulns(context.Background(), store, FetchOptions{
		Modules:   []string{"github.com/example/vuln"},
		GoVulnURL: goVulnSrv.URL,
		Quiet:     true,
	})
	if err != nil {
		t.Fatalf("FetchVulns: %v", err)
	}

	if result.ModulesChecked != 1 {
		t.Errorf("ModulesChecked: got %d, want 1", result.ModulesChecked)
	}
	if result.CVEsFound != 2 {
		t.Errorf("CVEsFound: got %d, want 2", result.CVEsFound)
	}
	if result.CVEsEnriched != 0 {
		t.Errorf("CVEsEnriched: got %d, want 0", result.CVEsEnriched)
	}

	// Verify data was stored in the database.
	entries, err := store.GetVulnCache("github.com/example/vuln")
	if err != nil {
		t.Fatalf("GetVulnCache: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 cached entries, got %d", len(entries))
	}

	// Verify CVE IDs were extracted from aliases (should prefer CVE- over GO-).
	cveIDs := make(map[string]bool)
	for _, e := range entries {
		cveIDs[e.CVEID] = true
	}
	if !cveIDs["CVE-2024-11111"] {
		t.Error("missing CVE-2024-11111 in cache")
	}
	if !cveIDs["CVE-2024-22222"] {
		t.Error("missing CVE-2024-22222 in cache")
	}

	// Verify severity defaults to UNKNOWN (no NVD enrichment).
	for _, e := range entries {
		if e.Severity != "UNKNOWN" {
			t.Errorf("CVE %s: expected severity UNKNOWN, got %q", e.CVEID, e.Severity)
		}
	}
}

func TestFetchVulns_WithNVDEnrichment(t *testing.T) {
	goVulnSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/vulns" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(makeGoVulnResponse(testGoVulnEntries()))
	}))
	defer goVulnSrv.Close()

	nvdSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API key header is present.
		if r.Header.Get("apiKey") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Return CVSS 8.1/HIGH for all CVEs.
		w.Write(makeNVDResponse("CVE-2024-11111", 8.1, "HIGH"))
	}))
	defer nvdSrv.Close()

	store := newTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result, err := FetchVulns(ctx, store, FetchOptions{
		Modules:   []string{"github.com/example/vuln"},
		GoVulnURL: goVulnSrv.URL,
		NVDURL:    nvdSrv.URL,
		NVDAPIKey: "test-key",
		NVDDelay:  -1, // skip rate-limit delay in tests
		Quiet:     true,
	})
	if err != nil {
		t.Fatalf("FetchVulns: %v", err)
	}

	if result.CVEsFound != 2 {
		t.Errorf("CVEsFound: got %d, want 2", result.CVEsFound)
	}
	if result.CVEsEnriched != 2 {
		t.Errorf("CVEsEnriched: got %d, want 2", result.CVEsEnriched)
	}

	// Verify severity was updated to HIGH via NVD enrichment.
	entries, err := store.GetVulnCache("github.com/example/vuln")
	if err != nil {
		t.Fatalf("GetVulnCache: %v", err)
	}
	for _, e := range entries {
		if e.Severity != "HIGH" {
			t.Errorf("CVE %s: expected severity HIGH after NVD enrichment, got %q", e.CVEID, e.Severity)
		}
	}
}

func TestFetchVulns_NoMatchingModules(t *testing.T) {
	goVulnSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(makeGoVulnResponse(testGoVulnEntries()))
	}))
	defer goVulnSrv.Close()

	store := newTestStore(t)
	result, err := FetchVulns(context.Background(), store, FetchOptions{
		Modules:   []string{"github.com/unrelated/module"},
		GoVulnURL: goVulnSrv.URL,
		Quiet:     true,
	})
	if err != nil {
		t.Fatalf("FetchVulns: %v", err)
	}

	if result.CVEsFound != 0 {
		t.Errorf("CVEsFound: got %d, want 0", result.CVEsFound)
	}
	if result.ModulesChecked != 1 {
		t.Errorf("ModulesChecked: got %d, want 1", result.ModulesChecked)
	}
}

func TestFetchVulns_EmptyModuleList(t *testing.T) {
	result, err := FetchVulns(context.Background(), nil, FetchOptions{
		Modules: []string{},
		Quiet:   true,
	})
	if err != nil {
		t.Fatalf("FetchVulns: unexpected error %v", err)
	}
	if result.ModulesChecked != 0 {
		t.Errorf("ModulesChecked: got %d, want 0", result.ModulesChecked)
	}
	if result.CVEsFound != 0 {
		t.Errorf("CVEsFound: got %d, want 0", result.CVEsFound)
	}
}

func TestExtractVersionRange(t *testing.T) {
	tests := []struct {
		name        string
		ranges      []osvRange
		wantAff     string
		wantFixed   string
	}{
		{
			name: "standard SEMVER range with introduced and fixed",
			ranges: []osvRange{
				{
					Type: "SEMVER",
					Events: []osvEvent{
						{Introduced: "1.0.0"},
						{Fixed: "1.5.0"},
					},
				},
			},
			wantAff:   ">= 1.0.0",
			wantFixed: "1.5.0",
		},
		{
			name: "introduced at zero",
			ranges: []osvRange{
				{
					Type: "SEMVER",
					Events: []osvEvent{
						{Introduced: "0"},
						{Fixed: "2.0.0"},
					},
				},
			},
			wantAff:   ">= 0",
			wantFixed: "2.0.0",
		},
		{
			name: "no fixed version",
			ranges: []osvRange{
				{
					Type: "SEMVER",
					Events: []osvEvent{
						{Introduced: "1.0.0"},
					},
				},
			},
			wantAff:   ">= 1.0.0",
			wantFixed: "",
		},
		{
			name: "non-SEMVER range is skipped",
			ranges: []osvRange{
				{
					Type: "GIT",
					Events: []osvEvent{
						{Introduced: "abc123"},
					},
				},
			},
			wantAff:   "",
			wantFixed: "",
		},
		{
			name:      "empty ranges",
			ranges:    []osvRange{},
			wantAff:   "",
			wantFixed: "",
		},
		{
			name: "SEMVER range with only fixed (no introduced)",
			ranges: []osvRange{
				{
					Type: "SEMVER",
					Events: []osvEvent{
						{Fixed: "3.0.0"},
					},
				},
			},
			wantAff:   "",
			wantFixed: "3.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAff, gotFixed := extractVersionRange(tt.ranges)
			if gotAff != tt.wantAff {
				t.Errorf("affected: got %q, want %q", gotAff, tt.wantAff)
			}
			if gotFixed != tt.wantFixed {
				t.Errorf("fixed: got %q, want %q", gotFixed, tt.wantFixed)
			}
		})
	}
}
