package derive

import (
	"testing"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

type imp = map[string]model.Impact

func TestAllOperational(t *testing.T) {
	s := testState(t)
	st := ComponentStatuses(s)
	if st["api"] != model.Operational || st["web"] != model.Operational || Overall(s) != model.Operational {
		t.Errorf("statuses = %v overall = %v", st, Overall(s))
	}
}

func TestWorstOpenEventWins(t *testing.T) {
	s := testState(t,
		event("2026-10-20-a", "A", model.KindIncident,
			upd(t0(20, 9, 0), model.Investigating, imp{"api": model.Degraded})),
		event("2026-10-20-b", "B", model.KindIncident,
			upd(t0(20, 10, 0), model.Investigating, imp{"api": model.MajorOutage, "web": model.PartialOutage}),
			upd(t0(20, 11, 0), model.Resolved, imp{"api": model.Operational, "web": model.Operational})),
		event("2026-10-21-c", "C", model.KindMaintenance,
			upd(t0(21, 9, 0), model.Scheduled, imp{"web": model.Operational}),
			upd(t0(21, 10, 0), model.InProgress, imp{"web": model.UnderMaintenance})),
	)
	st := ComponentStatuses(s)
	if st["api"] != model.Degraded {
		t.Errorf("resolved B must not count; api = %s", st["api"])
	}
	if st["web"] != model.UnderMaintenance {
		t.Errorf("in-progress maintenance must count; web = %s", st["web"])
	}
	if Overall(s) != model.Degraded {
		t.Errorf("overall = %s", Overall(s))
	}
	if got := ActiveEvents(s); len(got) != 2 || got[0].ID != "2026-10-21-c" {
		t.Errorf("ActiveEvents order wrong: %v", ids(got))
	}
}

func TestScheduledMaintenanceIsUpcomingNotActive(t *testing.T) {
	later := event("2026-10-21-later", "Later", model.KindMaintenance, upd(t0(21, 9, 0), model.Scheduled, imp{"web": model.Operational}))
	sooner := event("2026-10-22-sooner", "Sooner", model.KindMaintenance, upd(t0(22, 9, 0), model.Scheduled, imp{"web": model.Operational}))
	early := t0(21, 0, 0)
	sooner.ScheduledStart = &early
	s := testState(t, later, sooner)
	if Overall(s) != model.Operational || len(ActiveEvents(s)) != 0 {
		t.Error("scheduled maintenance must not affect status")
	}
	if got := Upcoming(s); len(got) != 2 || got[0].ID != "2026-10-22-sooner" {
		t.Errorf("Upcoming order wrong: %v", ids(got))
	}
}

func TestRetiredComponentIgnoredByOverall(t *testing.T) {
	s := testState(t, event("2026-10-20-ftp", "FTP down", model.KindIncident,
		upd(t0(20, 9, 0), model.Investigating, imp{"ftp": model.MajorOutage})))
	if Overall(s) != model.Operational {
		t.Errorf("retired component must not drive the banner, got %s", Overall(s))
	}
	if len(ActiveEvents(s)) != 1 {
		t.Error("the incident itself must still be listed as active")
	}
}

func TestRecentlyResolved(t *testing.T) {
	now := t0(23, 12, 0)
	recent := event("2026-10-20-r", "Recent", model.KindIncident,
		upd(t0(20, 9, 0), model.Investigating, imp{"api": model.Degraded}),
		upd(t0(20, 10, 0), model.Resolved, imp{"api": model.Operational}))
	old := event("2026-10-10-o", "Old", model.KindIncident,
		upd(t0(10, 9, 0), model.Investigating, imp{"api": model.Degraded}),
		upd(t0(10, 10, 0), model.Resolved, imp{"api": model.Operational}))
	open := event("2026-10-22-x", "Open", model.KindIncident, upd(t0(22, 9, 0), model.Investigating, imp{"api": model.Degraded}))
	got := RecentlyResolved(testState(t, old, recent, open), now, 7*24*time.Hour)
	if len(got) != 1 || got[0].ID != "2026-10-20-r" {
		t.Errorf("RecentlyResolved = %v", ids(got))
	}
}

func ids(ins []*model.Incident) []string {
	var out []string
	for _, in := range ins {
		out = append(out, in.ID)
	}
	return out
}

// A poster whose clock runs ahead of the build stamps the resolution in the
// future; the incident must still be listed, not vanish until a rebuild.
func TestRecentlyResolvedFutureStamp(t *testing.T) {
	now := t0(23, 12, 0)
	ahead := event("2026-10-23-f", "Ahead", model.KindIncident,
		upd(t0(23, 11, 0), model.Investigating, imp{"api": model.Degraded}),
		upd(t0(23, 12, 5), model.Resolved, imp{"api": model.Operational}))
	got := RecentlyResolved(testState(t, ahead), now, 7*24*time.Hour)
	if len(got) != 1 {
		t.Errorf("RecentlyResolved = %v, want the incident resolved 5 minutes 'in the future'", ids(got))
	}
}
