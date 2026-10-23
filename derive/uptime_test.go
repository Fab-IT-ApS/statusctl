package derive

import (
	"testing"
	"time"

	"statusctl/model"
)

var cph, _ = time.LoadLocation("Europe/Copenhagen")

func outage(id, title string, comp string, impact model.Impact, from, to time.Time) *model.Incident {
	return event(id, title, model.KindIncident,
		upd(from, model.Investigating, imp{comp: impact}),
		upd(to, model.Resolved, imp{comp: model.Operational}))
}

func TestUptimeNoIncidents(t *testing.T) {
	u := ComponentUptime(testState(t), "api", t0(23, 12, 0), 90)
	if len(u.Days) != 90 || u.Percent != 100 {
		t.Fatalf("len=%d pct=%v", len(u.Days), u.Percent)
	}
	if !u.Days[0].Date.Equal(time.Date(2026, 7, 26, 0, 0, 0, 0, cph)) {
		t.Errorf("first day = %v", u.Days[0].Date)
	}
	if !u.Days[89].Date.Equal(time.Date(2026, 10, 23, 0, 0, 0, 0, cph)) {
		t.Errorf("last day = %v", u.Days[89].Date)
	}
	for _, d := range u.Days {
		if d.Worst != model.Operational || len(d.Events) != 0 {
			t.Fatalf("day %v not clean", d.Date)
		}
	}
}

// Window: 2026-07-26 00:00 CEST (07-25 22:00Z) to 2026-10-23 12:00Z = 2150h.
func TestUptimeOneHourOutage(t *testing.T) {
	s := testState(t, outage("2026-10-20-down", "API down", "api", model.MajorOutage, t0(20, 10, 0), t0(20, 11, 0)))
	u := ComponentUptime(s, "api", t0(23, 12, 0), 90)
	d := u.Days[86] // 20 October
	if d.Worst != model.MajorOutage || len(d.Events) != 1 || d.Events[0] != "API down" {
		t.Errorf("day = %+v", d)
	}
	if u.Days[85].Worst != model.Operational || u.Days[87].Worst != model.Operational {
		t.Error("neighbouring days must stay operational")
	}
	if u.Percent != 99.95 {
		t.Errorf("percent = %v, want 99.95", u.Percent)
	}
	if web := ComponentUptime(s, "web", t0(23, 12, 0), 90); web.Percent != 100 {
		t.Error("other components must be unaffected")
	}
}

func TestUptimeOverlapCountsOnce(t *testing.T) {
	s := testState(t,
		outage("2026-10-20-a", "A", "api", model.PartialOutage, t0(20, 10, 0), t0(20, 12, 0)),
		outage("2026-10-20-b", "B", "api", model.PartialOutage, t0(20, 11, 0), t0(20, 13, 0)))
	u := ComponentUptime(s, "api", t0(23, 12, 0), 90)
	if u.Percent != 99.86 { // 3h of 2150h
		t.Errorf("percent = %v, want 99.86", u.Percent)
	}
	if len(u.Days[86].Events) != 2 {
		t.Errorf("both events must be listed: %v", u.Days[86].Events)
	}
}

func TestUptimeDegradedCountsAsUp(t *testing.T) {
	s := testState(t, outage("2026-10-20-slow", "Slow", "api", model.Degraded, t0(20, 10, 0), t0(20, 12, 0)))
	u := ComponentUptime(s, "api", t0(23, 12, 0), 90)
	if u.Percent != 100 || u.Days[86].Worst != model.Degraded {
		t.Errorf("percent=%v worst=%v", u.Percent, u.Days[86].Worst)
	}
}

func TestUptimeOpenIncidentRunsUntilNow(t *testing.T) {
	s := testState(t, event("2026-10-23-open", "Open", model.KindIncident,
		upd(t0(23, 11, 0), model.Investigating, imp{"api": model.MajorOutage})))
	u := ComponentUptime(s, "api", t0(23, 12, 0), 90)
	if u.Percent != 99.95 || u.Days[89].Worst != model.MajorOutage {
		t.Errorf("percent=%v today=%v", u.Percent, u.Days[89].Worst)
	}
}

func TestUptimeTinyOutageIsNot100(t *testing.T) {
	s := testState(t, outage("2026-10-20-blip", "Blip", "api", model.MajorOutage, t0(20, 10, 0), t0(20, 10, 1)))
	u := ComponentUptime(s, "api", t0(23, 12, 0), 90)
	if u.Percent != 99.99 {
		t.Errorf("a one-minute outage must show 99.99, got %v", u.Percent)
	}
}

func TestUptimeIgnoresEventsBeforeWindow(t *testing.T) {
	old := outage("2026-07-01-old", "Old", "api", model.MajorOutage,
		time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 1, 11, 0, 0, 0, time.UTC))
	u := ComponentUptime(testState(t, old), "api", t0(23, 12, 0), 90)
	if u.Percent != 100 {
		t.Errorf("percent = %v", u.Percent)
	}
}

func TestUptimeDSTDay(t *testing.T) {
	// DST ends 2026-10-25 03:00 CEST. 22:30Z on the 24th is 00:30 local on the 25th.
	s := testState(t, outage("2026-10-24-dst", "DST", "api", model.MajorOutage,
		time.Date(2026, 10, 24, 22, 30, 0, 0, time.UTC), time.Date(2026, 10, 24, 23, 30, 0, 0, time.UTC)))
	u := ComponentUptime(s, "api", time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC), 90)
	if got := u.Days[89].Date.Sub(u.Days[88].Date); got != 25*time.Hour {
		t.Errorf("25 October must last 25h, got %v", got)
	}
	if u.Days[88].Worst != model.MajorOutage || u.Days[87].Worst != model.Operational {
		t.Errorf("outage must land on 25 October local: 24=%v 25=%v", u.Days[87].Worst, u.Days[88].Worst)
	}
}
