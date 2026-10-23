package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"statusctl/model"
)

func loadRepo(t *testing.T, dir string) *model.State {
	t.Helper()
	s, err := model.Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIncidentLifecycle(t *testing.T) {
	dir := newRepo(t)
	r := run(t, "--dir", dir, "incident", "new", "API slow", "-c", "api=degraded", "-c", "web=partial_outage",
		"-m", "Investigating.", "--json")
	wantCode(t, r, 0, "")
	var out struct{ ID, Kind, Status, URL string }
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("%v: %q", err, r.stdout)
	}
	if out.ID != "2026-10-23-api-slow" || out.Status != "investigating" ||
		out.URL != "https://status.example.com/incidents/2026-10-23-api-slow/" {
		t.Errorf("json = %+v", out)
	}
	in := loadRepo(t, dir).Incident(out.ID)
	if in == nil || in.Latest().Author != "Git User" {
		t.Fatalf("incident = %+v", in)
	}

	wantCode(t, run(t, "--dir", dir, "incident", "update", "2026-10-23-api", "--status", "identified",
		"-c", "api=major_outage", "-m", "Found it.", "--author", "Bo"), 0, "Identified")
	wantCode(t, run(t, "--dir", dir, "incident", "resolve", "2026-10-23-api", "-m", "Fixed."), 0, "Resolved")
	in = loadRepo(t, dir).Incident(out.ID)
	if len(in.Updates) != 3 || in.Updates[1].Author != "Bo" || in.Latest().Impact["api"] != model.Operational {
		t.Errorf("timeline = %+v", in.Updates)
	}
	wantCode(t, run(t, "--dir", dir, "incident", "resolve", "2026-10-23-api", "-m", "Again."), 2, "--reopen")
}

func TestIncidentRejectsBeforeWriting(t *testing.T) {
	dir := newRepo(t)
	wantCode(t, run(t, "--dir", dir, "incident", "new", "X", "-c", "apii=degraded", "-m", "m"), 2, `unknown component "apii"`)
	wantCode(t, run(t, "--dir", dir, "incident", "new", "X", "-c", "api", "-m", "m"), 2, "ID=IMPACT")
	wantCode(t, run(t, "--dir", dir, "incident", "new", "X", "-c", "api=degraded", "-c", "api=major_outage", "-m", "m"), 2, "twice")
	wantCode(t, run(t, "--dir", dir, "incident", "new", "X", "-c", "api=degraded"), 2, "a message is required")
	wantCode(t, run(t, "--dir", dir, "incident", "new", "X", "-c", "api=degraded", "-m", "a", "-F", "f"), 2, "either -m or -F")
	wantCode(t, run(t, "--dir", dir, "incident", "new", "-c", "api=degraded", "-m", "m"), 2, "TITLE")
	if _, err := os.Stat(filepath.Join(dir, "incidents")); !os.IsNotExist(err) {
		t.Error("nothing may be written when input is invalid")
	}
}

func TestMessageFromStdinAndFile(t *testing.T) {
	dir := newRepo(t)
	a, out, errb := newTestApp(nil)
	a.stdin = strings.NewReader("From **stdin**.\n")
	wantCode(t, runApp(a, out, errb, "--dir", dir, "incident", "new", "A", "-c", "api=degraded", "-F", "-"), 0, "")

	msgFile := filepath.Join(t.TempDir(), "msg.md")
	if err := os.WriteFile(msgFile, []byte("From a file."), 0o644); err != nil {
		t.Fatal(err)
	}
	wantCode(t, run(t, "--dir", dir, "incident", "update", "2026-10-23-a", "-F", msgFile), 0, "")
	in := loadRepo(t, dir).Incident("2026-10-23-a")
	if in.Updates[0].Message != "From **stdin**." || in.Latest().Message != "From a file." {
		t.Errorf("messages = %q / %q", in.Updates[0].Message, in.Latest().Message)
	}
}

func TestMessageFromEditor(t *testing.T) {
	dir := newRepo(t)
	a, out, errb := newTestApp(map[string]string{"EDITOR": "myedit"})
	a.interactive = true
	var usedEditor string
	a.runEditor = func(editor, file string) error {
		usedEditor = editor
		body, _ := os.ReadFile(file)
		return os.WriteFile(file, append([]byte("Typed in the editor.\n"), body...), 0o600)
	}
	wantCode(t, runApp(a, out, errb, "--dir", dir, "incident", "new", "A", "-c", "api=degraded"), 0, "")
	if usedEditor != "myedit" || loadRepo(t, dir).Incident("2026-10-23-a").Latest().Message != "Typed in the editor." {
		t.Errorf("editor %q, message not taken from editor", usedEditor)
	}

	a, out, errb = newTestApp(nil)
	a.interactive = true
	a.runEditor = func(string, string) error { return nil } // leaves only the comment template
	wantCode(t, runApp(a, out, errb, "--dir", dir, "incident", "update", "2026-10-23-a"), 2, "empty message")
}

func TestAuthorRequired(t *testing.T) {
	dir := newRepo(t)
	a, out, errb := newTestApp(nil)
	a.gitUserName = func() string { return "" }
	wantCode(t, runApp(a, out, errb, "--dir", dir, "incident", "new", "A", "-c", "api=degraded", "-m", "m"), 2, "no author")
}

func TestMaintenanceLifecycle(t *testing.T) {
	dir := newRepo(t)
	wantCode(t, run(t, "--dir", dir, "maintenance", "new", "DB upgrade", "-c", "api", "-c", "web",
		"--start", "2026-11-02T22:00", "--end", "2026-11-03T00:00"), 0, "Scheduled")
	m := loadRepo(t, dir).Incident("2026-10-23-db-upgrade")
	if m == nil || m.ScheduledStart.Format("2006-01-02T15:04Z07:00") != "2026-11-02T21:00Z" {
		t.Fatalf("maintenance = %+v", m)
	}
	wantCode(t, run(t, "--dir", dir, "maintenance", "start", "2026-10-23-db"), 0, "In progress")
	wantCode(t, run(t, "--dir", dir, "maintenance", "complete", "2026-10-23-db", "-m", "Done."), 0, "Completed")
	wantCode(t, run(t, "--dir", dir, "maintenance", "start", "2026-10-23-db"), 2, "must be scheduled")
}

func TestMaintenanceRejects(t *testing.T) {
	dir := newRepo(t)
	wantCode(t, run(t, "--dir", dir, "maintenance", "new", "X", "-c", "api", "--start", "2026-11-02T22:00"), 2, "--start and --end")
	wantCode(t, run(t, "--dir", dir, "maintenance", "new", "X", "-c", "api", "--start", "soon", "--end", "later"), 2, "RFC 3339")
	wantCode(t, run(t, "--dir", dir, "maintenance", "new", "X", "-c", "api",
		"--start", "2026-11-02T22:00", "--end", "2026-11-02T21:00"), 2, "--end must be after --start")
	wantCode(t, run(t, "--dir", dir, "maintenance", "new", "X", "-c", "api",
		"--start", "2026-11-02T22:00", "--end", "2026-11-02T23:00"), 0, "")
	wantCode(t, run(t, "--dir", dir, "maintenance", "cancel", "2026-10-23-x"), 2, "a message is required")
	wantCode(t, run(t, "--dir", dir, "maintenance", "cancel", "2026-10-23-x", "-m", "Postponed."), 0, "Cancelled")
}

func editorWriting(text string) func(string, string) error {
	return func(_, file string) error {
		body, _ := os.ReadFile(file)
		return os.WriteFile(file, append([]byte(text), body...), 0o600)
	}
}

// Only the template's own comment lines are stripped; a user's line that
// happens to start with # is kept.
func TestEditorKeepsUserHashLines(t *testing.T) {
	dir := newRepo(t)
	a, out, errb := newTestApp(nil)
	a.interactive = true
	a.runEditor = editorWriting("#2 of the DB nodes is down.\n")
	wantCode(t, runApp(a, out, errb, "--dir", dir, "incident", "new", "A", "-c", "api=degraded"), 0, "")
	if got := loadRepo(t, dir).Incident("2026-10-23-a").Latest().Message; got != "#2 of the DB nodes is down." {
		t.Errorf("message = %q", got)
	}
}

// A message typed in the editor survives a failed post.
func TestEditorDraftSavedOnFailure(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	dir := newRepo(t)
	a, out, errb := newTestApp(nil)
	a.interactive = true
	a.runEditor = editorWriting("Long careful explanation.\n")
	r := runApp(a, out, errb, "--dir", dir, "incident", "update", "no-such-incident")
	wantCode(t, r, 2, "")
	_, after, ok := strings.Cut(r.stderr, "message saved to ")
	if !ok {
		t.Fatalf("stderr does not name a saved draft: %q", r.stderr)
	}
	path := strings.Fields(after)[0]
	body, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(body)) != "Long careful explanation." {
		t.Errorf("draft %s = %q, %v", path, body, err)
	}
}
