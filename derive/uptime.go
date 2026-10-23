package derive

import (
	"math"
	"slices"
	"time"

	"statusctl/model"
)

type Day struct {
	Date   time.Time // local midnight
	Worst  model.Impact
	Events []string // titles of events that affected the component that day
}

type Uptime struct {
	Days    []Day // oldest first
	Percent float64
}

type interval struct {
	start, end time.Time
	impact     model.Impact
	title      string
}

// ComponentUptime covers the last `days` local calendar days, ending today.
func ComponentUptime(s *model.State, id string, now time.Time, days int) Uptime {
	loc := s.Config.Location()
	n := now.In(loc)
	start := time.Date(n.Year(), n.Month(), n.Day()-(days-1), 0, 0, 0, 0, loc)
	ivs := intervals(s, id, now)

	u := Uptime{Days: make([]Day, 0, days)}
	for i := 0; i < days; i++ {
		d0 := time.Date(start.Year(), start.Month(), start.Day()+i, 0, 0, 0, 0, loc)
		d1 := time.Date(start.Year(), start.Month(), start.Day()+i+1, 0, 0, 0, 0, loc)
		if d1.After(now) {
			d1 = now
		}
		day := Day{Date: d0, Worst: model.Operational}
		for _, iv := range ivs {
			if iv.impact != model.Operational && iv.start.Before(d1) && iv.end.After(d0) {
				day.Worst = model.Worst(day.Worst, iv.impact)
				if !slices.Contains(day.Events, iv.title) {
					day.Events = append(day.Events, iv.title)
				}
			}
		}
		u.Days = append(u.Days, day)
	}

	total := now.Sub(start)
	raw := 100 * (1 - float64(downtime(ivs, start, now))/float64(total))
	u.Percent = math.Floor(raw*100) / 100
	return u
}

func intervals(s *model.State, id string, now time.Time) []interval {
	var out []interval
	for _, in := range s.Incidents {
		if !slices.Contains(in.Components, id) {
			continue
		}
		for i, u := range in.Updates {
			end := now
			if i+1 < len(in.Updates) {
				end = in.Updates[i+1].At
			} else if u.Status.Terminal() {
				continue
			}
			if end.After(u.At) {
				out = append(out, interval{start: u.At, end: end, impact: u.Impact[id], title: in.Title})
			}
		}
	}
	slices.SortFunc(out, func(a, b interval) int { return a.start.Compare(b.start) })
	return out
}

// downtime is the length of the union of down intervals, clipped to [from, to].
func downtime(ivs []interval, from, to time.Time) time.Duration {
	var total time.Duration
	var curStart, curEnd time.Time
	open := false
	for _, iv := range ivs {
		if !iv.impact.Down() {
			continue
		}
		s, e := iv.start, iv.end
		if s.Before(from) {
			s = from
		}
		if e.After(to) {
			e = to
		}
		if !e.After(s) {
			continue
		}
		if open && !s.After(curEnd) {
			if e.After(curEnd) {
				curEnd = e
			}
			continue
		}
		if open {
			total += curEnd.Sub(curStart)
		}
		curStart, curEnd, open = s, e, true
	}
	if open {
		total += curEnd.Sub(curStart)
	}
	return total
}
