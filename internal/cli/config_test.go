package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"statusctl/model"
	"statusctl/ops"
	"statusctl/store"
)

const profiles = `default_profile: acme
profiles:
  acme:
    repo: org/status-acme
    token_cmd: pass show github/status-acme
    author: Ada
  globex:
    repo: org/status-globex
    branch: live
`

func configFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProfileSelection(t *testing.T) {
	a, _, _ := newTestApp(nil)
	a.configPath = configFile(t, profiles)
	name, p, err := a.loadProfile()
	if err != nil || name != "acme" || p.Repo != "org/status-acme" || p.Branch != "main" {
		t.Errorf("default profile: %s %+v %v", name, p, err)
	}

	a, _, _ = newTestApp(map[string]string{"STATUSCTL_PROFILE": "globex"})
	a.configPath = configFile(t, profiles)
	if name, p, _ := a.loadProfile(); name != "globex" || p.Branch != "live" {
		t.Errorf("env profile: %s %+v", name, p)
	}

	a.profile = "acme"
	if name, _, _ := a.loadProfile(); name != "acme" {
		t.Error("--profile must beat STATUSCTL_PROFILE")
	}

	a.profile = "initech"
	_, _, err = a.loadProfile()
	var ie *ops.InputError
	if !errors.As(err, &ie) {
		t.Errorf("unknown profile must be an InputError, got %v", err)
	}
}

func TestProfileRejectsLiteralToken(t *testing.T) {
	a, _, _ := newTestApp(nil)
	a.configPath = configFile(t, "default_profile: x\nprofiles:\n  x:\n    repo: o/r\n    token: ghp_secret\n")
	_, _, err := a.loadProfile()
	var ie *ops.InputError
	if !errors.As(err, &ie) {
		t.Fatalf("a literal token field must be rejected, got %v", err)
	}
}

func TestToken(t *testing.T) {
	a, _, _ := newTestApp(map[string]string{"STATUSCTL_GITHUB_TOKEN": "from-env"})
	if tok, _ := a.token(profile{TokenCmd: "x"}); tok != "from-env" {
		t.Errorf("env must win, got %q", tok)
	}

	a, _, _ = newTestApp(nil)
	var ran string
	a.runShell = func(cmd string) (string, error) { ran = cmd; return "secret\n", nil }
	tok, err := a.token(profile{TokenCmd: "pass show x"})
	if err != nil || tok != "secret" || ran != "pass show x" {
		t.Errorf("token_cmd: %q %v (ran %q)", tok, err, ran)
	}

	a.runShell = func(string) (string, error) { return "  \n", nil }
	if _, err := a.token(profile{TokenCmd: "x"}); !errors.Is(err, store.ErrAuth) {
		t.Errorf("empty token_cmd output must be ErrAuth, got %v", err)
	}

	_, err = a.token(profile{})
	var ie *ops.InputError
	if !errors.As(err, &ie) {
		t.Errorf("no token source must be an InputError, got %v", err)
	}
}

type failingStore struct{ err error }

func (f failingStore) Load(context.Context) (*model.State, error) { return nil, f.err }
func (f failingStore) Apply(context.Context, store.Op) (*ops.Result, error) {
	return nil, f.err
}

// Spec §7: auth and network errors name the profile as well as the repo.
func TestStoreErrorsNameProfile(t *testing.T) {
	a, _, _ := newTestApp(nil)
	a.configPath = configFile(t, profiles)
	a.getenv = func(k string) string {
		if k == "STATUSCTL_GITHUB_TOKEN" {
			return "tok"
		}
		return ""
	}
	st, _, err := a.openStore()
	if err != nil {
		t.Fatal(err)
	}
	ps, ok := st.(profileStore)
	if !ok || ps.name != "acme" {
		t.Fatalf("openStore() = %T %+v, want profileStore for acme", st, st)
	}
	for _, sentinel := range []error{store.ErrAuth, store.ErrNetwork} {
		ps.Store = failingStore{fmt.Errorf("%w: GitHub returned 404 for org/status-acme", sentinel)}
		_, lerr := ps.Load(context.Background())
		_, aerr := ps.Apply(context.Background(), nil)
		for _, err := range []error{lerr, aerr} {
			if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), `profile "acme"`) {
				t.Errorf("got %v, want %v naming the profile", err, sentinel)
			}
		}
	}
	ps.Store = failingStore{&ops.InputError{Msg: "no such incident"}}
	if _, err := ps.Load(context.Background()); strings.Contains(err.Error(), "profile") {
		t.Errorf("input errors must stay unchanged, got %v", err)
	}
}
