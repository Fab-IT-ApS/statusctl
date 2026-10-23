package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testConfig = `site:
  name: Test Status
  url: "https://status.example.com"
  timezone: Europe/Copenhagen
components:
  - { id: api, name: Public API, group: Platform }
  - { id: web, name: Web portal, group: Platform }
  - { id: mail, name: Email }
  - { id: ftp, name: FTP, retired: true }
`

var testNow = time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)

type result struct {
	stdout, stderr string
	code           int
}

func newTestApp(env map[string]string) (*app, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	a := &app{
		stdout: &out, stderr: &errb, stdin: strings.NewReader(""),
		getenv:      func(k string) string { return env[k] },
		now:         func() time.Time { return testNow },
		runShell:    func(string) (string, error) { return "", errors.New("no shell in tests") },
		runEditor:   func(string, string) error { return errors.New("no editor in tests") },
		gitUserName: func() string { return "Git User" },
		httpGet:     func(string) ([]byte, error) { return nil, errors.New("no network in tests") },
	}
	return a, &out, &errb
}

func runApp(a *app, out, errb *bytes.Buffer, args ...string) result {
	code := a.run(args)
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}

func run(t *testing.T, args ...string) result {
	t.Helper()
	a, out, errb := newTestApp(nil)
	return runApp(a, out, errb, args...)
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "status.yaml"), []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func wantCode(t *testing.T, r result, code int, stderrHas string) {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit code = %d, want %d\nstdout: %s\nstderr: %s", r.code, code, r.stdout, r.stderr)
	}
	if stderrHas != "" && !strings.Contains(r.stderr, stderrHas) {
		t.Errorf("stderr %q does not mention %q", r.stderr, stderrHas)
	}
}
