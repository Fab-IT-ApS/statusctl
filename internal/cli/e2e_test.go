package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEndToEnd walks the onboarding and an incident through to a rendered site.
func TestEndToEnd(t *testing.T) {
	repo, site := t.TempDir(), t.TempDir()
	_, _, run := initApp(t)
	steps := [][]string{
		{"init", "--dir", repo, "--release", "v0.1.0", "--release-base", "https://x.example/dl"},
		{"--dir", repo, "incident", "new", "Website slow", "-c", "website=degraded", "-m", "Looking."},
		{"--dir", repo, "maintenance", "new", "Upgrade", "-c", "website", "--start", "2026-11-01T22:00", "--end", "2026-11-01T23:00"},
		{"--dir", repo, "incident", "resolve", "2026-10-23-website", "-m", "Fixed."},
		{"validate", "--dir", repo},
		{"render", "--dir", repo, "--out", site, "--now", "2026-10-23T12:00:00Z"},
	}
	for _, args := range steps {
		wantCode(t, run(args...), 0, "")
	}
	idx, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"All systems operational", "Upcoming maintenance", "Website slow", "Upgrade"} {
		if !strings.Contains(string(idx), want) {
			t.Errorf("index.html missing %q", want)
		}
	}
}
