package cli

import (
	"testing"
	"time"
)

// The daily rebuild must never fire before local midnight, whatever time of
// year init ran: it uses the zone's smallest UTC offset.
func TestDailyCronIgnoresSeasonOfInit(t *testing.T) {
	cases := []struct{ zone, want string }{
		{"Europe/Copenhagen", "15 23 * * *"},
		{"America/New_York", "15 5 * * *"},
		{"Australia/Sydney", "15 14 * * *"},
		{"UTC", "15 0 * * *"},
	}
	for _, c := range cases {
		loc, err := time.LoadLocation(c.zone)
		if err != nil {
			t.Fatal(err)
		}
		for _, now := range []time.Time{
			time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
			time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
		} {
			if got := dailyCron(loc, now); got != c.want {
				t.Errorf("dailyCron(%s, %s) = %q, want %q", c.zone, now.Format("Jan"), got, c.want)
			}
		}
	}
}
