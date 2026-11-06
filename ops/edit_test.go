package ops

import (
	"testing"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

func ptime(t time.Time) *time.Time { return &t }

func TestEditMessageAndTitle(t *testing.T) {
	s := testState(t)
	cur := openIncident(t, s)
	s.Incidents = []*model.Incident{cur}
	res, err := EditEvent(s, EditArgs{
		Ref: "2026-10-23-elev", Title: "API latency", Message: "Investigating slow responses.", Author: "Bo",
	})
	if err != nil {
		t.Fatal(err)
	}
	in, was := res.Incident, cur.Latest()
	if in.Title != "API latency" || in.ID != cur.ID || len(in.Updates) != 1 {
		t.Fatalf("edited = %+v", in)
	}
	u := in.Latest()
	if u.Message != "Investigating slow responses." || !u.At.Equal(was.At) || u.Status != was.Status ||
		u.Author != "Ada" || u.Impact["web"] != model.PartialOutage {
		t.Errorf("latest update = %+v", u)
	}
	if got := res.CommitMessage(); got != "incident 2026-10-23-elevated-api-latency: edited by Bo" {
		t.Errorf("CommitMessage = %q", got)
	}
	if got := res.CommitAuthorName(); got != "Bo" {
		t.Errorf("CommitAuthorName = %q", got)
	}
	if cur.Title != "Elevated API latency" {
		t.Error("EditEvent must not mutate loaded state")
	}
}

func TestEditOnlyLatestUpdate(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{openIncident(t, s)}
	up, err := UpdateIncident(s, UpdateArgs{Ref: "2026", Message: "Second.", Author: "Ada"}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	s.Incidents = []*model.Incident{up.Incident}
	res, err := EditEvent(s, EditArgs{Ref: "2026", Message: "Second, fixed.", Author: "Bo"})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Incident.Updates; m[0].Message != "Investigating." || m[1].Message != "Second, fixed." {
		t.Errorf("messages = %q / %q", m[0].Message, m[1].Message)
	}
}

func TestEditNothingToChange(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{openIncident(t, s)}
	_, err := EditEvent(s, EditArgs{Ref: "2026", Author: "Bo"})
	wantInputError(t, err, "nothing to change")
	_, err = EditEvent(s, EditArgs{Ref: "2026", Title: "Elevated API latency", Message: "  ", Author: "Bo"})
	wantInputError(t, err, "nothing to change")
}

func TestEditMaintenanceTimes(t *testing.T) {
	s := testState(t)
	m := scheduleDB(t, s)
	s.Incidents = []*model.Incident{m}
	res, err := EditEvent(s, EditArgs{Ref: "2026", End: ptime(time.Date(2026, 11, 2, 23, 30, 0, 0, time.UTC)), Author: "Bo"})
	if err != nil {
		t.Fatal(err)
	}
	if e := res.Incident.ScheduledEnd; !e.Equal(time.Date(2026, 11, 2, 23, 30, 0, 0, time.UTC)) {
		t.Errorf("end = %v", e)
	}
	if !res.Incident.ScheduledStart.Equal(*m.ScheduledStart) {
		t.Errorf("start changed to %v", res.Incident.ScheduledStart)
	}
	_, err = EditEvent(s, EditArgs{Ref: "2026", End: ptime(time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC)), Author: "Bo"})
	wantInputError(t, err, "end must be after the start")
}

func TestEditTimesRefused(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{openIncident(t, s)}
	_, err := EditEvent(s, EditArgs{Ref: "2026", Start: ptime(now), Author: "Bo"})
	wantInputError(t, err, "only maintenance")

	s.Incidents = []*model.Incident{scheduleDB(t, s)}
	started, err := StartMaintenance(s, "2026", "", "Ada", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	s.Incidents = []*model.Incident{started.Incident}
	_, err = EditEvent(s, EditArgs{Ref: "2026", End: ptime(now.Add(48 * time.Hour)), Author: "Bo"})
	wantInputError(t, err, "before it starts")
}

func TestEditKindMismatch(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{scheduleDB(t, s)}
	_, err := EditEvent(s, EditArgs{Ref: "2026", Kind: model.KindIncident, Message: "x", Author: "Bo"})
	wantInputError(t, err, "use the maintenance commands")
	s.Incidents = []*model.Incident{openIncident(t, s)}
	_, err = EditEvent(s, EditArgs{Ref: "2026", Kind: model.KindMaintenance, Message: "x", Author: "Bo"})
	wantInputError(t, err, "use the incident commands")
}

func TestEditValidates(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{openIncident(t, s)}
	_, err := EditEvent(s, EditArgs{Ref: "2026", Title: "two\nlines", Author: "Bo"})
	wantInputError(t, err, "one line")
	_, err = EditEvent(s, EditArgs{Ref: "2026", Message: "x"})
	wantInputError(t, err, "an author is required")
}
