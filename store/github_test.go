package store

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"statusctl/model"
	"statusctl/ops"
)

var ctx = context.Background()

func baseFiles() map[string]string { return map[string]string{"status.yaml": testConfig} }

func otherIncidentJSON(t *testing.T) string {
	t.Helper()
	in := &model.Incident{ID: "2026-10-22-other", Kind: model.KindIncident, Title: "Other", Components: []string{"web"},
		Updates: []model.Update{{At: testNow.Add(-24 * time.Hour), Status: model.Investigating, Message: "m", Author: "Bo",
			Impact: map[string]model.Impact{"web": model.Degraded}}}}
	b, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNewGitHubValidatesRepo(t *testing.T) {
	for _, bad := range []string{"", "noslash", "/r", "o/", "o/r/x"} {
		_, err := NewGitHub(bad, "main", "tok")
		var ie *ops.InputError
		if !errors.As(err, &ie) {
			t.Errorf("NewGitHub(%q) should be an InputError, got %v", bad, err)
		}
	}
}

func TestGitHubLoad(t *testing.T) {
	files := baseFiles()
	files["incidents/2026-10-22-other.json"] = otherIncidentJSON(t)
	_, g := newFake(t, files)
	s, err := g.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Config.Components) != 2 || len(s.Incidents) != 1 {
		t.Errorf("state = %d components, %d incidents", len(s.Config.Components), len(s.Incidents))
	}
}

func TestGitHubApplyCommits(t *testing.T) {
	f, g := newFake(t, baseFiles())
	res, err := g.Apply(ctx, newIncidentOp("API slow", nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.headFiles()["incidents/"+res.Incident.ID+".json"]; !ok {
		t.Fatal("incident file not in the new head")
	}
	_, c := f.headCommit()
	if c.message != "incident 2026-10-23-api-slow: investigating" {
		t.Errorf("message = %q", c.message)
	}
	if c.author["name"] != "Ada" || c.author["email"] != "7+octo@users.noreply.github.com" {
		t.Errorf("author = %v", c.author)
	}
	if c.parent != "c0" {
		t.Errorf("parent = %q", c.parent)
	}
}

func TestGitHubRetriesOnConflict(t *testing.T) {
	f, g := newFake(t, baseFiles())
	other := otherIncidentJSON(t)
	f.beforePatch = func(f *fakeGitHub, n int) {
		if n == 0 {
			f.pushDirect(map[string]string{"incidents/2026-10-22-other.json": other})
		}
	}
	calls := 0
	if _, err := g.Apply(ctx, newIncidentOp("API slow", &calls)); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || f.patchCount() != 2 {
		t.Errorf("op calls = %d, patches = %d; want 2 and 2", calls, f.patchCount())
	}
	head := f.headFiles()
	if _, ok := head["incidents/2026-10-22-other.json"]; !ok {
		t.Error("the concurrent write was lost")
	}
	if _, ok := head["incidents/2026-10-23-api-slow.json"]; !ok {
		t.Error("our write is missing")
	}
}

func TestGitHubGivesUp(t *testing.T) {
	f, g := newFake(t, baseFiles())
	f.beforePatch = func(f *fakeGitHub, n int) {
		f.pushDirect(map[string]string{fmt.Sprintf("notes/%d.txt", n): "x"})
	}
	_, err := g.Apply(ctx, newIncidentOp("API slow", nil))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	if f.patchCount() != 5 {
		t.Errorf("patches = %d, want 5", f.patchCount())
	}
}

func TestGitHubAuthError(t *testing.T) {
	_, g := newFake(t, baseFiles())
	g.Token = "wrong"
	_, err := g.Load(ctx)
	if !errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), "o/r") {
		t.Fatalf("want ErrAuth naming the repo, got %v", err)
	}
}

func TestGitHubInputErrorNotRetried(t *testing.T) {
	f, g := newFake(t, baseFiles())
	want := &ops.InputError{Msg: "bad"}
	calls := 0
	_, err := g.Apply(ctx, func(*model.State) (*ops.Result, error) { calls++; return nil, want })
	head, _ := f.headCommit()
	if !errors.Is(err, want) || calls != 1 || f.patchCount() != 0 || head != "c0" {
		t.Errorf("err=%v calls=%d patches=%d head=%s", err, calls, f.patchCount(), head)
	}
}

func TestGitHubWithoutUserEndpoint(t *testing.T) {
	f, g := newFake(t, baseFiles())
	f.userStatus = http.StatusForbidden
	if _, err := g.Apply(ctx, newIncidentOp("API slow", nil)); err != nil {
		t.Fatal(err)
	}
	if _, c := f.headCommit(); c.author != nil {
		t.Errorf("author must be omitted when /user is unavailable, got %v", c.author)
	}
}

// A 422 on the ref update with the branch head unchanged is a policy refusal
// (branch protection, rulesets), not a race: fail at once with GitHub's reason.
func TestGitHubRuleRejectionNotRetried(t *testing.T) {
	f, g := newFake(t, baseFiles())
	f.refRule = "Repository rule violations found: Changes must be made through a pull request."
	_, err := g.Apply(ctx, newIncidentOp("API slow", nil))
	if err == nil || errors.Is(err, ErrConflict) {
		t.Fatalf("want a non-conflict error, got %v", err)
	}
	if !errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), "must be made through a pull request") {
		t.Errorf("want ErrAuth carrying GitHub's message, got %v", err)
	}
	if f.patchCount() != 1 {
		t.Errorf("patches = %d, want 1", f.patchCount())
	}
}

func TestGitHubUnknownBranchNamed(t *testing.T) {
	_, g := newFake(t, baseFiles())
	g.Branch = "mian"
	_, err := g.Load(ctx)
	if !errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), `branch "mian"`) {
		t.Fatalf("want an error naming the branch, got %v", err)
	}
}

// A failed tarball redirect must not print the short-lived download token.
func TestGitHubNetworkErrorRedactsQuery(t *testing.T) {
	_, g := newFake(t, baseFiles())
	g.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset")
	})
	g.BaseURL = "https://api.example.invalid/x?token=SECRET"
	_, err := g.Load(ctx)
	if !errors.Is(err, ErrNetwork) || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("want ErrNetwork without the query, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
