package tools

import (
	"strings"
	"testing"

	"github.com/kengou/go-guardian/mcp-server/db"
)

func TestComputeDirection(t *testing.T) {
	cases := []struct {
		name   string
		counts []int64 // most recent first
		want   string
	}{
		{
			name:   "improving: 10 → 7 → 5 (newest=5, oldest=10)",
			counts: []int64{5, 7, 10},
			want:   "IMPROVING",
		},
		{
			name:   "degrading: 5 → 7 → 10 (newest=10, oldest=5)",
			counts: []int64{10, 7, 5},
			want:   "DEGRADING",
		},
		{
			name:   "stable: 5 → 5 → 5",
			counts: []int64{5, 5, 5},
			want:   "STABLE",
		},
		{
			name:   "mixed: 10 → 5 → 8 (no clear trend)",
			counts: []int64{8, 5, 10},
			want:   "STABLE",
		},
		{
			name:   "insufficient: single snapshot",
			counts: []int64{5},
			want:   "no trend (insufficient data)",
		},
		{
			name:   "two snapshots improving",
			counts: []int64{3, 8},
			want:   "IMPROVING",
		},
		{
			name:   "two snapshots degrading",
			counts: []int64{8, 3},
			want:   "DEGRADING",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshots := make([]db.ScanSnapshot, len(tc.counts))
			for i, c := range tc.counts {
				snapshots[i] = db.ScanSnapshot{FindingsCount: c}
			}
			got := computeDirection(snapshots)
			if got != tc.want {
				t.Errorf("computeDirection(%v) = %q, want %q", tc.counts, got, tc.want)
			}
		})
	}
}

func TestComputeRecurring(t *testing.T) {
	snapshots := []db.ScanSnapshot{
		{FindingsDetail: `{"categories":{"errcheck":3,"govet":1}}`},
		{FindingsDetail: `{"categories":{"errcheck":2,"staticcheck":1}}`},
		{FindingsDetail: `{"categories":{"errcheck":1}}`},
		{FindingsDetail: `{}`},
	}

	result := computeRecurring(snapshots)
	if len(result) == 0 {
		t.Fatal("expected recurring issues")
	}
	if result[0].name != "errcheck" {
		t.Errorf("top recurring: want errcheck, got %q", result[0].name)
	}
	if result[0].count != 3 {
		t.Errorf("errcheck count: want 3, got %d", result[0].count)
	}
}

func TestGetHealthTrendsEmpty(t *testing.T) {
	store := newTestStore(t)

	text, err := RunGetHealthTrends(store, "proj", "")
	if err != nil {
		t.Fatalf("RunGetHealthTrends: %v", err)
	}
	if !strings.Contains(text, "No scan history") {
		t.Errorf("expected empty-state message, got: %q", text)
	}
}

func TestGetHealthTrendsFormatting(t *testing.T) {
	store := newTestStore(t)

	// Seed snapshots: improving lint trend.
	_ = store.InsertScanSnapshot("lint", "proj", 15, `{"categories":{"errcheck":5}}`)
	_ = store.InsertScanSnapshot("lint", "proj", 10, `{"categories":{"errcheck":3}}`)
	_ = store.InsertScanSnapshot("lint", "proj", 5, `{"categories":{"errcheck":1}}`)

	// Stable owasp trend.
	_ = store.InsertScanSnapshot("owasp", "proj", 3, `{}`)
	_ = store.InsertScanSnapshot("owasp", "proj", 3, `{}`)

	text, err := RunGetHealthTrends(store, "proj", "")
	if err != nil {
		t.Fatalf("RunGetHealthTrends: %v", err)
	}

	// Check key sections present.
	for _, substr := range []string{"Health Trends", "lint:", "owasp:", "Overall:", "errcheck"} {
		if !strings.Contains(text, substr) {
			t.Errorf("output missing %q:\n%s", substr, text)
		}
	}
}

func TestGetHealthTrendsFilterByScanType(t *testing.T) {
	store := newTestStore(t)

	_ = store.InsertScanSnapshot("lint", "proj", 5, `{}`)
	_ = store.InsertScanSnapshot("owasp", "proj", 3, `{}`)

	text, err := RunGetHealthTrends(store, "proj", "lint")
	if err != nil {
		t.Fatalf("RunGetHealthTrends: %v", err)
	}

	if !strings.Contains(text, "lint:") {
		t.Errorf("expected lint in output: %q", text)
	}
	// owasp should NOT appear since we filtered to lint.
	if strings.Contains(text, "owasp:") {
		t.Errorf("owasp should not appear in lint-filtered output: %q", text)
	}
}
