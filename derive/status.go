// Package derive computes everything that is not stored: component status,
// the overall banner, event lists and uptime.
package derive

import (
	"slices"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

// Active reports whether an event currently affects component status.
func Active(in *model.Incident) bool {
	st := in.Latest().Status
	if in.Kind == model.KindIncident {
		return !st.Terminal()
	}
	return st == model.InProgress
}

// ComponentStatuses is the worst impact of every active event on each component.
func ComponentStatuses(s *model.State) map[string]model.Impact {
	out := make(map[string]model.Impact, len(s.Config.Components))
	for _, c := range s.Config.Components {
		out[c.ID] = model.Operational
	}
	for _, in := range s.Incidents {
		if !Active(in) {
			continue
		}
		for id, imp := range in.Latest().Impact {
			out[id] = model.Worst(out[id], imp)
		}
	}
	return out
}

// Overall is the worst status across non-retired components.
func Overall(s *model.State) model.Impact {
	st := ComponentStatuses(s)
	worst := model.Operational
	for _, c := range s.Config.ActiveComponents() {
		worst = model.Worst(worst, st[c.ID])
	}
	return worst
}

func ActiveEvents(s *model.State) []*model.Incident {
	var out []*model.Incident
	for _, in := range s.Incidents {
		if Active(in) {
			out = append(out, in)
		}
	}
	slices.SortStableFunc(out, func(a, b *model.Incident) int { return b.Started().Compare(a.Started()) })
	return out
}

func Upcoming(s *model.State) []*model.Incident {
	var out []*model.Incident
	for _, in := range s.Incidents {
		if in.Kind == model.KindMaintenance && in.Latest().Status == model.Scheduled {
			out = append(out, in)
		}
	}
	slices.SortStableFunc(out, func(a, b *model.Incident) int { return a.ScheduledStart.Compare(*b.ScheduledStart) })
	return out
}

func RecentlyResolved(s *model.State, now time.Time, window time.Duration) []*model.Incident {
	cutoff := now.Add(-window)
	var out []*model.Incident
	for _, in := range s.Incidents {
		l := in.Latest()
		if in.Kind == model.KindIncident && l.Status == model.Resolved && !l.At.Before(cutoff) {
			out = append(out, in)
		}
	}
	slices.SortStableFunc(out, func(a, b *model.Incident) int { return b.LastUpdated().Compare(a.LastUpdated()) })
	return out
}
