package model

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func baseIncident() *Incident {
	at := time.Date(2026, 10, 23, 9, 12, 0, 0, time.UTC)
	return &Incident{
		ID: "2026-10-23-api-latency", Kind: KindIncident, Title: "Elevated API latency",
		Components: []string{"api"},
		Updates: []Update{{At: at, Status: Investigating, Message: "Looking into it.", Author: "Ops",
			Impact: map[string]Impact{"api": Degraded}}},
	}
}

func repoFS(t *testing.T, incidents ...*Incident) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{
		"status.yaml":     {Data: []byte(validConfig)},
		"assets/logo.svg": {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)},
	}
	for _, in := range incidents {
		b, err := in.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		fsys[in.Path()] = &fstest.MapFile{Data: b}
	}
	return fsys
}

func wantProblem(t *testing.T, err error, want string) {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *ValidationError mentioning %q, got %v", want, err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not mention %q", err, want)
	}
}

func TestLoadEmptyRepo(t *testing.T) {
	s, err := Load(repoFS(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Incidents) != 0 || len(s.Config.Components) != 3 {
		t.Errorf("got %d incidents, %d components", len(s.Incidents), len(s.Config.Components))
	}
}

func TestLoadRoundTrip(t *testing.T) {
	in := baseIncident()
	s, err := Load(repoFS(t, in))
	if err != nil {
		t.Fatal(err)
	}
	got := s.Incident(in.ID)
	if got == nil || got.Title != in.Title || got.Latest().Impact["api"] != Degraded {
		t.Fatalf("round trip lost data: %+v", got)
	}
	if !got.Started().Equal(in.Updates[0].At) || !got.LastUpdated().Equal(in.Updates[0].At) {
		t.Error("Started/LastUpdated wrong")
	}
}

func TestLoadAllowsRetiredComponentInHistory(t *testing.T) {
	in := baseIncident()
	in.Components = []string{"ftp"}
	in.Updates[0].Impact = map[string]Impact{"ftp": MajorOutage}
	if _, err := Load(repoFS(t, in)); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingConfig(t *testing.T) {
	_, err := Load(fstest.MapFS{})
	wantProblem(t, err, "status.yaml")
}

func TestLoadMissingLogo(t *testing.T) {
	fsys := repoFS(t)
	delete(fsys, "assets/logo.svg")
	_, err := Load(fsys)
	wantProblem(t, err, `site.logo "assets/logo.svg" does not exist`)
}

func TestLoadRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Incident)
		want   string
	}{
		{"deleted component", func(in *Incident) {
			in.Components = []string{"gone"}
			in.Updates[0].Impact = map[string]Impact{"gone": Degraded}
		}, "retired: true"},
		{"impact missing", func(in *Incident) { in.Updates[0].Impact = map[string]Impact{} }, "impact must give an impact"},
		{"impact for unlisted component", func(in *Incident) { in.Updates[0].Impact["web"] = Degraded }, `names "web"`},
		{"bad impact value", func(in *Incident) { in.Updates[0].Impact["api"] = "meh" }, "not a valid impact"},
		{"status for wrong kind", func(in *Incident) { in.Updates[0].Status = Scheduled }, "not valid for incident"},
		{"empty message", func(in *Incident) { in.Updates[0].Message = "  " }, "message must be"},
		{"message too long", func(in *Incident) { in.Updates[0].Message = strings.Repeat("x", MaxMessage+1) }, "message must be"},
		{"no author", func(in *Incident) { in.Updates[0].Author = "" }, "author is required"},
		{"multi-line title", func(in *Incident) { in.Title = "a\nb" }, "title must be one line"},
		{"updates out of order", func(in *Incident) {
			u := in.Updates[0]
			u.At = u.At.Add(-time.Hour)
			in.Updates = append(in.Updates, u)
		}, "earlier than the update before it"},
		{"maintenance without schedule", func(in *Incident) {
			in.Kind = KindMaintenance
			in.Updates[0].Status = Scheduled
		}, "needs scheduled_start and scheduled_end"},
		{"incident with schedule", func(in *Incident) {
			at := in.Updates[0].At
			in.ScheduledStart, in.ScheduledEnd = &at, &at
		}, "only maintenance has a schedule"},
		{"no updates", func(in *Incident) { in.Updates = nil }, "updates must not be empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := baseIncident()
			c.mutate(in)
			_, err := Load(repoFS(t, in))
			wantProblem(t, err, c.want)
		})
	}
}

func TestLoadRejectsFileNameMismatch(t *testing.T) {
	fsys := repoFS(t)
	b, _ := baseIncident().Marshal()
	fsys["incidents/2026-10-23-other.json"] = &fstest.MapFile{Data: b}
	_, err := Load(fsys)
	wantProblem(t, err, "does not match the file name")
}

func TestLoadRejectsUnknownJSONField(t *testing.T) {
	fsys := repoFS(t)
	fsys["incidents/2026-10-23-x.json"] = &fstest.MapFile{Data: []byte(`{"id":"2026-10-23-x","severity":"high"}`)}
	_, err := Load(fsys)
	wantProblem(t, err, `unknown field "severity"`)
}

func TestLoadReportsEveryBrokenFile(t *testing.T) {
	a, b := baseIncident(), baseIncident()
	b.ID = "2026-10-23-second"
	a.Updates[0].Author = ""
	b.Updates[0].Author = ""
	_, err := Load(repoFS(t, a, b))
	wantProblem(t, err, "incidents/2026-10-23-api-latency.json")
	wantProblem(t, err, "incidents/2026-10-23-second.json")
}

func TestLoadIgnoresNonJSON(t *testing.T) {
	fsys := repoFS(t)
	fsys["incidents/.gitkeep"] = &fstest.MapFile{}
	if _, err := Load(fsys); err != nil {
		t.Fatal(err)
	}
}

func TestCloneIsDeep(t *testing.T) {
	in := baseIncident()
	c := in.Clone()
	c.Updates[0].Impact["api"] = MajorOutage
	c.Components[0] = "web"
	c.Updates = append(c.Updates, Update{})
	if in.Updates[0].Impact["api"] != Degraded || in.Components[0] != "api" || len(in.Updates) != 1 {
		t.Error("Clone shares memory with the original")
	}
}
