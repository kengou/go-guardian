module github.com/go-guardian/e2e-fixture

go 1.23

// Pinned to v0.3.6 — has CVE-2021-38561 (GO-2021-0113):
// Out-of-bounds read in golang.org/x/text/language.
// Fixed in v0.3.7. Do NOT upgrade — this is intentional for e2e testing.
require golang.org/x/text v0.3.6
