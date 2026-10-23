package cli

import (
	"context"
	"encoding/json"
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

func seedIncident(t *testing.T, dir, title string) string {
	t.Helper()
	res, err := store.Local{Dir: dir}.Apply(context.Background(), func(s *model.State) (*ops.Result, error) {
		return ops.NewIncident(s, ops.NewIncidentArgs{Title: title,
			Impacts: map[string]model.Impact{"api": model.MajorOutage}, Message: "m", Author: "Ada"}, testNow)
	})
	if err != nil {
		t.Fatal(err)
	}
	return res.Incident.ID
}

func TestValidate(t *testing.T) {
	dir := newRepo(t)
	wantCode(t, run(t, "validate", "--dir", dir), 0, "ok: 4 components")

	if err := os.WriteFile(filepath.Join(dir, "status.yaml"), []byte(strings.Replace(testConfig, "Europe/Copenhagen", "Nowhere/Land", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	wantCode(t, run(t, "validate", "--dir", dir), 2, "timezone")
}

func TestRender(t *testing.T) {
	dir := newRepo(t)
	seedIncident(t, dir, "API down")
	out := t.TempDir()
	wantCode(t, run(t, "render", "--dir", dir, "--out", out, "--now", "2026-10-23T12:00:00Z"), 0, "rendered")
	for _, f := range []string{"index.html", "feed.atom", "api/v1/status.json", "incidents/2026-10-23-api-down/index.html", "_headers"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(f))); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
	}
	wantCode(t, run(t, "render", "--dir", dir), 2, "--out is required")
	wantCode(t, run(t, "render", "--dir", dir, "--out", out, "--now", "yesterday"), 2, "--now")
}

func TestStatusJSON(t *testing.T) {
	dir := newRepo(t)
	seedIncident(t, dir, "API down")
	r := run(t, "status", "--dir", dir, "--json")
	wantCode(t, r, 0, "")
	var got struct {
		Status     model.Impact
		Components []struct {
			ID     string
			Status model.Impact
		}
	}
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("%v: %s", err, r.stdout)
	}
	if got.Status != model.MajorOutage || len(got.Components) != 3 || got.Components[0].Status != model.MajorOutage {
		t.Errorf("status = %+v", got)
	}
}

func TestStatusHuman(t *testing.T) {
	dir := newRepo(t)
	r := run(t, "status", "--dir", dir)
	wantCode(t, r, 0, "")
	if !strings.HasPrefix(r.stdout, "Operational\n") || !strings.Contains(r.stdout, "Public API") {
		t.Errorf("stdout = %q", r.stdout)
	}
}

func TestList(t *testing.T) {
	dir := newRepo(t)
	id := seedIncident(t, dir, "API down")
	r := run(t, "list", "--dir", dir, "--open", "--json")
	wantCode(t, r, 0, "")
	var got []struct{ ID, Kind, Status, Title string }
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("%v: %s", err, r.stdout)
	}
	if len(got) != 1 || got[0].ID != id || got[0].Status != "investigating" {
		t.Errorf("list = %+v", got)
	}
	r = run(t, "list", "--dir", dir)
	if !strings.Contains(r.stdout, id) || !strings.Contains(r.stdout, "API down") {
		t.Errorf("human list = %q", r.stdout)
	}
}

func TestUnknownCommandAndFlag(t *testing.T) {
	wantCode(t, run(t, "frobnicate"), 2, "unknown command")
	wantCode(t, run(t, "status", "--bogus"), 2, "unknown flag")
	wantCode(t, run(t, "validate", "extra"), 2, "expects")
}

func TestNoConfigWithoutDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yaml")
	wantCode(t, run(t, "status", "--config", missing), 2, "no config file")
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{errors.New("boom"), 1},
		{&ops.InputError{Msg: "x"}, 2},
		{&model.ValidationError{Problems: []string{"x"}}, 2},
		{fmt.Errorf("wrapped: %w", store.ErrConflict), 3},
		{fmt.Errorf("%w: bad token", store.ErrAuth), 4},
		{fmt.Errorf("%w: timeout", store.ErrNetwork), 4},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}
