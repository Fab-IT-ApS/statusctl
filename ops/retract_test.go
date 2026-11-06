package ops

import (
	"strings"
	"testing"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

func resolvedAt(t *testing.T, s *model.State, at time.Time) *model.Incident {
	t.Helper()
	s.Incidents = []*model.Incident{openIncident(t, s)}
	res, err := ResolveIncident(s, "2026", "Fixed.", "Ada", at)
	if err != nil {
		t.Fatal(err)
	}
	return res.Incident
}

func TestRetractOpen(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{openIncident(t, s)}
	res, err := RetractEvent(s, RetractArgs{Ref: "2026", Author: "Bo"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Deleted) != 1 || res.Deleted[0] != "incidents/2026-10-23-elevated-api-latency.json" || len(res.Files) != 0 {
		t.Errorf("deleted = %v, files = %v", res.Deleted, res.Files)
	}
	if res.URL != "https://status.example.com/" {
		t.Errorf("URL = %q", res.URL)
	}
	if !strings.HasSuffix(res.CommitMessage(), "retracted by Bo") || res.CommitAuthorName() != "Bo" {
		t.Errorf("commit = %q by %q", res.CommitMessage(), res.CommitAuthorName())
	}
	if res.Incident.Title != "Elevated API latency" {
		t.Errorf("result must describe the retracted event, got %+v", res.Incident)
	}
}

func TestRetractRecentlyClosed(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{resolvedAt(t, s, now)}
	if _, err := RetractEvent(s, RetractArgs{Ref: "2026", Author: "Bo"}, now.Add(RetractWindow)); err != nil {
		t.Fatal(err)
	}
}

func TestRetractOldRefused(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{resolvedAt(t, s, now)}
	late := now.Add(7*24*time.Hour + time.Second)
	_, err := RetractEvent(s, RetractArgs{Ref: "2026", Author: "Bo"}, late)
	wantInputError(t, err, "more than 7 days ago")
	if _, err := RetractEvent(s, RetractArgs{Ref: "2026", Author: "Bo", Force: true}, late); err != nil {
		t.Fatal(err)
	}
}

func TestRetractOpenNeverExpires(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{openIncident(t, s)}
	if _, err := RetractEvent(s, RetractArgs{Ref: "2026", Author: "Bo"}, now.Add(30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func TestRetractKindMismatch(t *testing.T) {
	s := testState(t)
	s.Incidents = []*model.Incident{scheduleDB(t, s)}
	_, err := RetractEvent(s, RetractArgs{Ref: "2026", Kind: model.KindIncident, Author: "Bo"}, now)
	wantInputError(t, err, "use the maintenance commands")
	_, err = RetractEvent(s, RetractArgs{Ref: "2026", Kind: model.KindMaintenance}, now)
	wantInputError(t, err, "an author is required")
}
