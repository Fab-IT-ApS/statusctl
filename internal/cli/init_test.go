package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func initApp(t *testing.T) (*app, *[]string, func(args ...string) result) {
	t.Helper()
	a, out, errb := newTestApp(nil)
	var fetched []string
	a.httpGet = func(url string) ([]byte, error) {
		fetched = append(fetched, url)
		return []byte(sum + "  statusctl_linux_amd64\n" + strings.Repeat("f", 64) + "  statusctl_darwin_arm64\n"), nil
	}
	return a, &fetched, func(args ...string) result { out.Reset(); errb.Reset(); return runApp(a, out, errb, args...) }
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	_, fetched, run := initApp(t)
	wantCode(t, run("init", "--dir", dir, "--release", "v0.1.0", "--release-base", "https://github.com/org/statusctl/releases/download/"), 0, "")
	if len(*fetched) != 1 || (*fetched)[0] != "https://github.com/org/statusctl/releases/download/v0.1.0/SHA256SUMS" {
		t.Errorf("fetched %v", *fetched)
	}
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	toml := read("netlify.toml")
	for _, want := range []string{`publish = "site"`, `command = "sh ./netlify-build.sh"`,
		`STATUSCTL_VERSION = "v0.1.0"`, `STATUSCTL_SHA256 = "` + sum + `"`,
		`STATUSCTL_URL = "https://github.com/org/statusctl/releases/download"`} {
		if !strings.Contains(toml, want) {
			t.Errorf("netlify.toml missing %q:\n%s", want, toml)
		}
	}
	script := read("netlify-build.sh")
	for _, want := range []string{"sha256sum -c -", "statusctl_linux_amd64", "render --dir . --out site"} {
		if !strings.Contains(script, want) {
			t.Errorf("netlify-build.sh missing %q", want)
		}
	}
	wf := read(".github/workflows/daily-rebuild.yml")
	for _, want := range []string{`cron: "15 23 * * *"`, "${{ secrets.NETLIFY_BUILD_HOOK }}", "permissions: {}"} {
		if !strings.Contains(wf, want) {
			t.Errorf("workflow missing %q:\n%s", want, wf)
		}
	}
	if read(".gitignore") != "site/\n" {
		t.Error(".gitignore wrong")
	}
	wantCode(t, run("validate", "--dir", dir), 0, "ok:")
}

func TestInitRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "status.yaml"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, run := initApp(t)
	wantCode(t, run("init", "--dir", dir, "--release", "v0.1.0", "--release-base", "https://x.example/dl"), 2, "status.yaml already exists")
	if _, err := os.Stat(filepath.Join(dir, "netlify.toml")); !os.IsNotExist(err) {
		t.Error("init must write nothing when it refuses")
	}
}

func TestInitInputs(t *testing.T) {
	_, _, run := initApp(t)
	wantCode(t, run("init", "--dir", t.TempDir(), "--release-base", "https://x.example/dl"), 2, "development build")
	wantCode(t, run("init", "--dir", t.TempDir(), "--release", "v0.1.0"), 2, "--release-base")
	wantCode(t, run("init", "--dir", t.TempDir(), "--release", `v1"; rm -rf /`, "--release-base", "https://x.example/dl"), 2, "--release")
	wantCode(t, run("init", "--dir", t.TempDir(), "--release", "v0.1.0", "--release-base", "http://x.example/dl"), 2, "https")
	wantCode(t, run("init", "--dir", t.TempDir(), "--release", "v0.1.0", "--release-base", "https://x.example/dl", "--timezone", "Mars/Base"), 2, "timezone")
}

func TestInitBadChecksums(t *testing.T) {
	a, _, run := initApp(t)
	a.httpGet = func(string) ([]byte, error) { return []byte("nothing useful\n"), nil }
	wantCode(t, run("init", "--dir", t.TempDir(), "--release", "v0.1.0", "--release-base", "https://x.example/dl"), 4, "statusctl_linux_amd64")
}
