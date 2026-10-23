package ops

import (
	"testing"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

func scheduleDB(t *testing.T, s *model.State) *model.Incident {
	t.Helper()
	res, err := NewMaintenance(s, NewMaintenanceArgs{
		Title: "DB upgrade", Components: []string{"web", "api"},
		Start:  time.Date(2026, 11, 2, 21, 0, 0, 0, time.UTC),
		End:    time.Date(2026, 11, 2, 23, 0, 0, 0, time.UTC),
		Author: "Ada",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return res.Incident
}

func TestMaintenanceLifecycle(t *testing.T) {
	s := testState(t)
	m := scheduleDB(t, s)
	if m.Kind != model.KindMaintenance || m.Latest().Status != model.Scheduled || m.Latest().Message != "Maintenance scheduled" {
		t.Fatalf("new maintenance wrong: %+v", m.Latest())
	}
	if m.Components[0] != "api" || m.Latest().Impact["api"] != model.Operational {
		t.Errorf("scheduled maintenance must list components in config order with operational impact: %+v", m)
	}
	s.Incidents = []*model.Incident{m}

	res, err := StartMaintenance(s, "2026-10-23-db", "", "Ada", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if u := res.Incident.Latest(); u.Status != model.InProgress || u.Impact["web"] != model.UnderMaintenance || u.Message != "Maintenance has started" {
		t.Fatalf("start wrong: %+v", u)
	}
	s.Incidents = []*model.Incident{res.Incident}

	res, err = CompleteMaintenance(s, "2026-10-23-db", "All done.", "Ada", now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if u := res.Incident.Latest(); u.Status != model.Completed || u.Impact["web"] != model.Operational || u.Message != "All done." {
		t.Fatalf("complete wrong: %+v", u)
	}
	if res.CommitMessage() != "maintenance 2026-10-23-db-upgrade: completed" {
		t.Errorf("CommitMessage = %q", res.CommitMessage())
	}
}

func TestCancelMaintenance(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{scheduleDB(t, s)}
	_, err := CancelMaintenance(s, "2026", "", "Ada", now)
	wantInputError(t, err, "message is required to cancel")
	res, err := CancelMaintenance(s, "2026", "Postponed.", "Ada", now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Incident.Latest().Status != model.Cancelled {
		t.Error("cancel wrong")
	}
}

func TestMaintenanceRejects(t *testing.T) {
	s := testState(t)
	m := scheduleDB(t, s)
	s.Incidents = []*model.Incident{m}

	_, err := CompleteMaintenance(s, "2026", "", "Ada", now)
	wantInputError(t, err, "must be in_progress")

	_, err = NewMaintenance(s, NewMaintenanceArgs{Title: "x", Components: []string{"api"},
		Start: now, End: now, Author: "a"}, now)
	wantInputError(t, err, "--end must be after --start")

	_, err = NewMaintenance(s, NewMaintenanceArgs{Title: "x", Components: []string{"api", "api"},
		Start: now, End: now.Add(time.Hour), Author: "a"}, now)
	wantInputError(t, err, "listed twice")

	_, err = NewMaintenance(s, NewMaintenanceArgs{Title: "x", Start: now, End: now.Add(time.Hour), Author: "a"}, now)
	wantInputError(t, err, "at least one affected component")

	inc := openIncident(t, s)
	s.Incidents = append(s.Incidents, inc)
	_, err = StartMaintenance(s, inc.ID, "", "Ada", now)
	wantInputError(t, err, "is an incident")
	_, err = UpdateIncident(s, UpdateArgs{Ref: m.ID, Message: "m", Author: "a"}, now)
	wantInputError(t, err, "maintenance window")
}

func TestParseTime(t *testing.T) {
	cph, _ := time.LoadLocation("Europe/Copenhagen")
	got, err := ParseTime("2026-11-02T22:00", cph)
	if err != nil || !got.Equal(time.Date(2026, 11, 2, 21, 0, 0, 0, time.UTC)) {
		t.Errorf("local time: got %v %v", got, err)
	}
	got, err = ParseTime("2026-11-02T22:00:00Z", cph)
	if err != nil || !got.Equal(time.Date(2026, 11, 2, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("RFC 3339: got %v %v", got, err)
	}
	_, err = ParseTime("tomorrow", cph)
	wantInputError(t, err, "RFC 3339")
}
