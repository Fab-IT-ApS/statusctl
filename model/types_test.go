package model

import "testing"

func TestWorst(t *testing.T) {
	cases := []struct{ a, b, want Impact }{
		{Operational, Degraded, Degraded},
		{MajorOutage, PartialOutage, MajorOutage},
		{UnderMaintenance, Degraded, Degraded},
		{UnderMaintenance, Operational, UnderMaintenance},
		{"", Degraded, Degraded},
	}
	for _, c := range cases {
		if got := Worst(c.a, c.b); got != c.want {
			t.Errorf("Worst(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

func TestImpactHelpers(t *testing.T) {
	if !PartialOutage.Down() || !MajorOutage.Down() || Degraded.Down() || UnderMaintenance.Down() {
		t.Error("Down() must be true only for partial_outage and major_outage")
	}
	if Impact("broken").Valid() || !Degraded.Valid() {
		t.Error("Valid() wrong")
	}
	if MajorOutage.Label() != "Major outage" || UnderMaintenance.Label() != "Under maintenance" {
		t.Error("Label() wrong")
	}
}

func TestStatusRules(t *testing.T) {
	if !Investigating.ValidFor(KindIncident) || Investigating.ValidFor(KindMaintenance) {
		t.Error("investigating is incident-only")
	}
	if !InProgress.ValidFor(KindMaintenance) || InProgress.ValidFor(KindIncident) {
		t.Error("in_progress is maintenance-only")
	}
	for _, s := range []Status{Resolved, Completed, Cancelled} {
		if !s.Terminal() {
			t.Errorf("%s must be terminal", s)
		}
	}
	for _, s := range []Status{Investigating, Identified, Monitoring, Scheduled, InProgress} {
		if s.Terminal() {
			t.Errorf("%s must not be terminal", s)
		}
	}
	if InProgress.Label() != "In progress" {
		t.Error("Label() wrong")
	}
}
