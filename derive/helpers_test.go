package derive

import (
	"maps"
	"slices"
	"testing"
	"time"

	"statusctl/model"
)

const testConfig = `site:
  name: Test Status
  url: "https://status.example.com"
  timezone: Europe/Copenhagen
components:
  - { id: api, name: Public API }
  - { id: web, name: Web portal }
  - { id: ftp, name: FTP, retired: true }
`

func testState(t *testing.T, incidents ...*model.Incident) *model.State {
	t.Helper()
	cfg, err := model.ParseConfig([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	return &model.State{Config: cfg, Incidents: incidents}
}

func upd(at time.Time, st model.Status, impact map[string]model.Impact) model.Update {
	return model.Update{At: at, Status: st, Message: "m", Author: "a", Impact: impact}
}

// event builds an incident (or, with kind maintenance, a window) whose
// components are the keys of the first update's impact.
func event(id, title string, kind model.Kind, updates ...model.Update) *model.Incident {
	in := &model.Incident{ID: id, Kind: kind, Title: title, Updates: updates,
		Components: slices.Sorted(maps.Keys(updates[0].Impact))}
	if kind == model.KindMaintenance {
		start, end := updates[0].At.Add(24*time.Hour), updates[0].At.Add(26*time.Hour)
		in.ScheduledStart, in.ScheduledEnd = &start, &end
	}
	return in
}

func t0(day, hour, min int) time.Time { return time.Date(2026, 10, day, hour, min, 0, 0, time.UTC) }
