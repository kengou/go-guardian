package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// TestUpdateOWASPRules_StoresFindings — two advisories with known CWEs
// ---------------------------------------------------------------------------

func TestUpdateOWASPRules_StoresFindings(t *testing.T) {
	advisories := []ghsaAdvisory{
		{
			GHSAID:  "GHSA-1111-1111-1111",
			CVEID:   "CVE-2024-0001",
			Summary: "SQL injection in example/db",
			CWEs: []struct {
				CWEID string `json:"cwe_id"`
				Name  string `json:"name"`
			}{
				{CWEID: "CWE-89", Name: "SQL Injection"},
			},
			Vulnerabilities: []struct {
				Package struct {
					Ecosystem string `json:"ecosystem"`
					Name      string `json:"name"`
				} `json:"package"`
				VulnerableVersionRange string `json:"vulnerable_version_range"`
				FirstPatchedVersion    *struct {
					Identifier string `json:"identifier"`
				} `json:"first_patched_version"`
			}{
				{
					Package: struct {
						Ecosystem string `json:"ecosystem"`
						Name      string `json:"name"`
					}{Ecosystem: "Go", Name: "github.com/example/db"},
					VulnerableVersionRange: "< 1.2.0",
					FirstPatchedVersion: &struct {
						Identifier string `json:"identifier"`
					}{Identifier: "1.2.0"},
				},
			},
		},
		{
			GHSAID:  "GHSA-2222-2222-2222",
			CVEID:   "CVE-2024-0002",
			Summary: "SSRF in example/http",
			CWEs: []struct {
				CWEID string `json:"cwe_id"`
				Name  string `json:"name"`
			}{
				{CWEID: "CWE-918", Name: "Server-Side Request Forgery"},
			},
			Vulnerabilities: []struct {
				Package struct {
					Ecosystem string `json:"ecosystem"`
					Name      string `json:"name"`
				} `json:"package"`
				VulnerableVersionRange string `json:"vulnerable_version_range"`
				FirstPatchedVersion    *struct {
					Identifier string `json:"identifier"`
				} `json:"first_patched_version"`
			}{
				{
					Package: struct {
						Ecosystem string `json:"ecosystem"`
						Name      string `json:"name"`
					}{Ecosystem: "Go", Name: "github.com/example/http"},
					VulnerableVersionRange: "< 2.0.0",
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		if page == "" || page == "1" {
			json.NewEncoder(w).Encode(advisories)
			return
		}
		// Page 2+: empty array to stop pagination.
		json.NewEncoder(w).Encode([]ghsaAdvisory{})
	}))
	defer srv.Close()

	store := newTestStore(t)

	result, err := UpdateOWASPRules(context.Background(), store, OWASPUpdateOptions{
		GHSAURL:   srv.URL,
		PageDelay: -1, // skip rate-limit delay in tests
		Quiet:     true,
	})
	if err != nil {
		t.Fatalf("UpdateOWASPRules: %v", err)
	}

	if result.AdvisoriesFetched != 2 {
		t.Errorf("AdvisoriesFetched = %d, want 2", result.AdvisoriesFetched)
	}
	if result.PatternsStored != 2 {
		t.Errorf("PatternsStored = %d, want 2", result.PatternsStored)
	}

	// Verify A03-Injection was stored.
	if count, ok := result.CategoriesUpdated["A03-Injection"]; !ok || count != 1 {
		t.Errorf("A03-Injection count = %d, want 1", count)
	}
	// Verify A10-SSRF was stored.
	if count, ok := result.CategoriesUpdated["A10-SSRF"]; !ok || count != 1 {
		t.Errorf("A10-SSRF count = %d, want 1", count)
	}

	// Verify owasp_findings were actually persisted in the DB.
	findings, err := store.QueryOWASPFindings("example", 10)
	if err != nil {
		t.Fatalf("QueryOWASPFindings: %v", err)
	}
	if len(findings) != 2 {
		t.Errorf("stored findings = %d, want 2", len(findings))
	}
}

// ---------------------------------------------------------------------------
// TestUpdateOWASPRules_SkipsUnknownCWEs — advisory with unmapped CWE
// ---------------------------------------------------------------------------

func TestUpdateOWASPRules_SkipsUnknownCWEs(t *testing.T) {
	advisories := []ghsaAdvisory{
		{
			GHSAID:  "GHSA-9999-9999-9999",
			CVEID:   "CVE-2024-9999",
			Summary: "Unknown vulnerability type",
			CWEs: []struct {
				CWEID string `json:"cwe_id"`
				Name  string `json:"name"`
			}{
				{CWEID: "CWE-9999", Name: "Unknown CWE"},
			},
			Vulnerabilities: []struct {
				Package struct {
					Ecosystem string `json:"ecosystem"`
					Name      string `json:"name"`
				} `json:"package"`
				VulnerableVersionRange string `json:"vulnerable_version_range"`
				FirstPatchedVersion    *struct {
					Identifier string `json:"identifier"`
				} `json:"first_patched_version"`
			}{
				{
					Package: struct {
						Ecosystem string `json:"ecosystem"`
						Name      string `json:"name"`
					}{Ecosystem: "Go", Name: "github.com/example/unknown"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			json.NewEncoder(w).Encode([]ghsaAdvisory{})
			return
		}
		json.NewEncoder(w).Encode(advisories)
	}))
	defer srv.Close()

	store := newTestStore(t)

	result, err := UpdateOWASPRules(context.Background(), store, OWASPUpdateOptions{
		GHSAURL:   srv.URL,
		PageDelay: -1,
		Quiet:     true,
	})
	if err != nil {
		t.Fatalf("UpdateOWASPRules: %v", err)
	}

	if result.AdvisoriesFetched != 1 {
		t.Errorf("AdvisoriesFetched = %d, want 1", result.AdvisoriesFetched)
	}
	if result.PatternsStored != 0 {
		t.Errorf("PatternsStored = %d, want 0", result.PatternsStored)
	}
}

// ---------------------------------------------------------------------------
// TestUpdateOWASPRules_UpdatesScanHistory — scan_history row for owasp_rules
// ---------------------------------------------------------------------------

func TestUpdateOWASPRules_UpdatesScanHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]ghsaAdvisory{})
	}))
	defer srv.Close()

	store := newTestStore(t)

	_, err := UpdateOWASPRules(context.Background(), store, OWASPUpdateOptions{
		GHSAURL:   srv.URL,
		PageDelay: -1,
		Quiet:     true,
	})
	if err != nil {
		t.Fatalf("UpdateOWASPRules: %v", err)
	}

	history, err := store.GetScanHistory("global")
	if err != nil {
		t.Fatalf("GetScanHistory: %v", err)
	}

	found := false
	for _, h := range history {
		if h.ScanType == "owasp_rules" {
			found = true
			break
		}
	}
	if !found {
		t.Error("scan_history missing row for type=owasp_rules")
	}
}

// ---------------------------------------------------------------------------
// TestUpdateOWASPRules_EmptyResponse — empty GHSA response, no error
// ---------------------------------------------------------------------------

func TestUpdateOWASPRules_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ghsaAdvisory{})
	}))
	defer srv.Close()

	store := newTestStore(t)

	result, err := UpdateOWASPRules(context.Background(), store, OWASPUpdateOptions{
		GHSAURL:   srv.URL,
		PageDelay: -1,
		Quiet:     true,
	})
	if err != nil {
		t.Fatalf("UpdateOWASPRules: %v", err)
	}

	if result.AdvisoriesFetched != 0 {
		t.Errorf("AdvisoriesFetched = %d, want 0", result.AdvisoriesFetched)
	}
	if result.PatternsStored != 0 {
		t.Errorf("PatternsStored = %d, want 0", result.PatternsStored)
	}
	if len(result.CategoriesUpdated) != 0 {
		t.Errorf("CategoriesUpdated has %d entries, want 0", len(result.CategoriesUpdated))
	}
}

// ---------------------------------------------------------------------------
// TestCWEsToOWASPCategories — unit test the CWE mapping function
// ---------------------------------------------------------------------------

func TestCWEsToOWASPCategories(t *testing.T) {
	type cweEntry struct {
		CWEID string `json:"cwe_id"`
		Name  string `json:"name"`
	}

	cases := []struct {
		name string
		cwes []cweEntry
		want []string
	}{
		{
			name: "single known CWE",
			cwes: []cweEntry{{CWEID: "CWE-89", Name: "SQL Injection"}},
			want: []string{"A03-Injection"},
		},
		{
			name: "multiple CWEs same category",
			cwes: []cweEntry{
				{CWEID: "CWE-89", Name: "SQL Injection"},
				{CWEID: "CWE-79", Name: "XSS"},
			},
			want: []string{"A03-Injection"},
		},
		{
			name: "multiple CWEs different categories",
			cwes: []cweEntry{
				{CWEID: "CWE-89", Name: "SQL Injection"},
				{CWEID: "CWE-918", Name: "SSRF"},
			},
			want: []string{"A03-Injection", "A10-SSRF"},
		},
		{
			name: "unknown CWE only",
			cwes: []cweEntry{{CWEID: "CWE-9999", Name: "Unknown"}},
			want: nil,
		},
		{
			name: "empty CWEs",
			cwes: nil,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert to the anonymous struct type expected by cwesToOWASPCategories.
			input := make([]struct {
				CWEID string `json:"cwe_id"`
				Name  string `json:"name"`
			}, len(tc.cwes))
			for i, c := range tc.cwes {
				input[i].CWEID = c.CWEID
				input[i].Name = c.Name
			}

			got := cwesToOWASPCategories(input)

			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}

			gotSet := make(map[string]bool)
			for _, g := range got {
				gotSet[g] = true
			}
			for _, w := range tc.want {
				if !gotSet[w] {
					t.Errorf("missing category %q in result %v", w, got)
				}
			}
		})
	}
}
