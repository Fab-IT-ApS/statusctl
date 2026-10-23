package ops

import (
	"strings"
	"testing"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

func TestNewIncident(t *testing.T) {
	s := testState(t)
	res, err := NewIncident(s, NewIncidentArgs{
		Title:   "  Elevated API latency ",
		Impacts: map[string]model.Impact{"web": model.PartialOutage, "api": model.Degraded},
		Message: "Investigating.", Author: "Ada",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	in := res.Incident
	if in.ID != "2026-10-23-elevated-api-latency" || in.Title != "Elevated API latency" {
		t.Errorf("id/title = %q/%q", in.ID, in.Title)
	}
	if strings.Join(in.Components, ",") != "api,web" {
		t.Errorf("components must follow status.yaml order, got %v", in.Components)
	}
	u := in.Latest()
	if u.Status != model.Investigating || !u.At.Equal(now.Truncate(time.Second)) || u.Impact["web"] != model.PartialOutage {
		t.Errorf("update wrong: %+v", u)
	}
	if _, ok := res.Files["incidents/2026-10-23-elevated-api-latency.json"]; !ok || len(res.Files) != 1 {
		t.Errorf("files = %v", res.Files)
	}
	if res.URL != "https://status.example.com/incidents/2026-10-23-elevated-api-latency/" {
		t.Errorf("URL = %q", res.URL)
	}
	if res.CommitMessage() != "incident 2026-10-23-elevated-api-latency: investigating" {
		t.Errorf("CommitMessage = %q", res.CommitMessage())
	}
	if len(s.Incidents) != 0 {
		t.Error("NewIncident must not mutate the state")
	}
}

func TestNewIncidentIDs(t *testing.T) {
	s := testState(t)
	first := openIncident(t, s)
	s.Incidents = append(s.Incidents, first)
	second := openIncident(t, s)
	if second.ID != "2026-10-23-elevated-api-latency-2" {
		t.Errorf("collision id = %q", second.ID)
	}
	for title, want := range map[string]string{
		"Nedbrud på e-mail":               "2026-10-23-nedbrud-paa-e-mail",
		"Æble Øl Å":                       "2026-10-23-aeble-oel-aa",
		"!!!":                             "2026-10-23-event",
		strings.Repeat("abcdefghij ", 10): "2026-10-23-abcdefghij-abcdefghij-abcdefghij-abcdefg",
	} {
		if got := NewID(testState(t), title, now); got != want {
			t.Errorf("NewID(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestNewIncidentRejects(t *testing.T) {
	valid := func() NewIncidentArgs {
		return NewIncidentArgs{Title: "x", Impacts: map[string]model.Impact{"api": model.Degraded}, Message: "m", Author: "a"}
	}
	cases := []struct {
		name   string
		mutate func(*NewIncidentArgs)
		want   string
	}{
		{"unknown component", func(a *NewIncidentArgs) { a.Impacts = map[string]model.Impact{"apii": model.Degraded} }, `unknown component "apii"`},
		{"retired component", func(a *NewIncidentArgs) { a.Impacts = map[string]model.Impact{"ftp": model.Degraded} }, "retired"},
		{"maintenance impact", func(a *NewIncidentArgs) { a.Impacts["api"] = model.UnderMaintenance }, "impact"},
		{"bogus impact", func(a *NewIncidentArgs) { a.Impacts["api"] = "down" }, "impact"},
		{"no components", func(a *NewIncidentArgs) { a.Impacts = nil }, "at least one affected component"},
		{"resolved status", func(a *NewIncidentArgs) { a.Status = model.Resolved }, "investigating, identified or monitoring"},
		{"blank title", func(a *NewIncidentArgs) { a.Title = "  " }, "title"},
		{"two-line title", func(a *NewIncidentArgs) { a.Title = "a\nb" }, "title"},
		{"empty message", func(a *NewIncidentArgs) { a.Message = " " }, "message is required"},
		{"long message", func(a *NewIncidentArgs) { a.Message = strings.Repeat("x", model.MaxMessage+1) }, "longer than"},
		{"no author", func(a *NewIncidentArgs) { a.Author = "" }, "author"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := valid()
			c.mutate(&a)
			_, err := NewIncident(testState(t), a, now)
			wantInputError(t, err, c.want)
		})
	}
}

func TestUpdateIncident(t *testing.T) {
	s := testState(t)
	s.Incidents = append(s.Incidents, openIncident(t, s))
	later := now.Add(10 * time.Minute)
	res, err := UpdateIncident(s, UpdateArgs{
		Ref: "2026-10-23-elev", Status: model.Identified,
		Impacts: map[string]model.Impact{"api": model.MajorOutage}, Message: "Found it.", Author: "Bo",
	}, later)
	if err != nil {
		t.Fatal(err)
	}
	u := res.Incident.Latest()
	if len(res.Incident.Updates) != 2 || u.Status != model.Identified || u.Author != "Bo" {
		t.Fatalf("update wrong: %+v", u)
	}
	if u.Impact["api"] != model.MajorOutage || u.Impact["web"] != model.PartialOutage {
		t.Errorf("impacts must be overridden or carried forward: %v", u.Impact)
	}
	if len(s.Incidents[0].Updates) != 1 {
		t.Error("UpdateIncident must not mutate the loaded incident")
	}

	res, err = UpdateIncident(s, UpdateArgs{Ref: "2026-10-23-elevated-api-latency", Message: "Still on it.", Author: "Bo"}, later)
	if err != nil {
		t.Fatal(err)
	}
	if res.Incident.Latest().Status != model.Investigating {
		t.Errorf("status must carry forward when omitted, got %s", res.Incident.Latest().Status)
	}
}

func TestUpdateIncidentClockBehind(t *testing.T) {
	s := testState(t)
	s.Incidents = append(s.Incidents, openIncident(t, s))
	res, err := UpdateIncident(s, UpdateArgs{Ref: "2026", Message: "m", Author: "a"}, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if res.Incident.Latest().At.Before(res.Incident.Updates[0].At) {
		t.Error("an update must never be stamped before the previous one")
	}
}

func TestResolveAndReopen(t *testing.T) {
	s := testState(t)
	s.Incidents = append(s.Incidents, openIncident(t, s))
	res, err := ResolveIncident(s, "2026-10-23", "Fixed.", "Ada", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	u := res.Incident.Latest()
	if u.Status != model.Resolved || u.Impact["api"] != model.Operational || u.Impact["web"] != model.Operational {
		t.Fatalf("resolve must clear impacts: %+v", u)
	}
	s.Incidents[0] = res.Incident

	_, err = ResolveIncident(s, "2026-10-23", "Again.", "Ada", now.Add(2*time.Hour))
	wantInputError(t, err, "--reopen")

	res, err = UpdateIncident(s, UpdateArgs{Ref: "2026-10-23", Reopen: true,
		Impacts: map[string]model.Impact{"api": model.Degraded}, Message: "Back.", Author: "Ada"}, now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if res.Incident.Latest().Status != model.Investigating || res.Incident.Latest().Impact["api"] != model.Degraded {
		t.Errorf("reopen wrong: %+v", res.Incident.Latest())
	}
}

func TestUpdateIncidentRejects(t *testing.T) {
	s := testState(t)
	s.Incidents = append(s.Incidents, openIncident(t, s))
	cases := []struct {
		name string
		args UpdateArgs
		want string
	}{
		{"component not in incident", UpdateArgs{Ref: "2026", Impacts: map[string]model.Impact{"mail": model.Degraded}, Message: "m", Author: "a"}, "not part of"},
		{"resolve with impacts", UpdateArgs{Ref: "2026", Status: model.Resolved, Impacts: map[string]model.Impact{"api": model.Degraded}, Message: "m", Author: "a"}, "drop the -c"},
		{"maintenance status", UpdateArgs{Ref: "2026", Status: model.InProgress, Message: "m", Author: "a"}, "must be investigating"},
		{"unknown ref", UpdateArgs{Ref: "2025", Message: "m", Author: "a"}, "no incident or maintenance matches"},
		{"empty ref", UpdateArgs{Message: "m", Author: "a"}, "ID is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := UpdateIncident(s, c.args, now)
			wantInputError(t, err, c.want)
		})
	}
}

func TestFindAmbiguous(t *testing.T) {
	s := testState(t)
	a := openIncident(t, s)
	s.Incidents = append(s.Incidents, a)
	b := openIncident(t, s)
	s.Incidents = append(s.Incidents, b)
	_, err := Find(s, "2026-10-23-elev")
	wantInputError(t, err, "ambiguous")
	got, err := Find(s, a.ID)
	if err != nil || got != a {
		t.Errorf("exact ID must win over prefix matches: %v %v", got, err)
	}
}
