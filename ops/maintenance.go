package ops

import (
	"strings"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

const (
	msgScheduled = "Maintenance scheduled"
	msgStarted   = "Maintenance has started"
	msgCompleted = "Maintenance completed"
)

type NewMaintenanceArgs struct {
	Title      string
	Components []string
	Start, End time.Time
	Message    string // optional; defaults to "Maintenance scheduled"
	Author     string
}

func NewMaintenance(s *model.State, a NewMaintenanceArgs, now time.Time) (*Result, error) {
	title, err := checkTitle(a.Title)
	if err != nil {
		return nil, err
	}
	if len(a.Components) == 0 {
		return nil, inputErrorf("name at least one affected component with -c ID")
	}
	set := map[string]bool{}
	for _, id := range a.Components {
		if err := checkComponent(s.Config, id); err != nil {
			return nil, err
		}
		if set[id] {
			return nil, inputErrorf("component %q is listed twice", id)
		}
		set[id] = true
	}
	if !a.End.After(a.Start) {
		return nil, inputErrorf("--end must be after --start")
	}
	msg, err := messageOrDefault(a.Message, msgScheduled)
	if err != nil {
		return nil, err
	}
	if err := checkAuthor(a.Author); err != nil {
		return nil, err
	}
	ids := orderedComponents(s.Config, set)
	start, end := stamp(a.Start), stamp(a.End)
	in := &model.Incident{
		ID: NewID(s, title, now), Kind: model.KindMaintenance, Title: title, Components: ids,
		ScheduledStart: &start, ScheduledEnd: &end,
		Updates: []model.Update{{
			At: stamp(now), Status: model.Scheduled, Message: msg,
			Author: strings.TrimSpace(a.Author), Impact: uniformImpact(ids, model.Operational),
		}},
	}
	return finish(s, in)
}

func StartMaintenance(s *model.State, ref, message, author string, now time.Time) (*Result, error) {
	return moveMaintenance(s, ref, model.Scheduled, model.InProgress, model.UnderMaintenance, message, msgStarted, author, now)
}

func CompleteMaintenance(s *model.State, ref, message, author string, now time.Time) (*Result, error) {
	return moveMaintenance(s, ref, model.InProgress, model.Completed, model.Operational, message, msgCompleted, author, now)
}

func CancelMaintenance(s *model.State, ref, message, author string, now time.Time) (*Result, error) {
	if strings.TrimSpace(message) == "" {
		return nil, inputErrorf("a message is required to cancel maintenance")
	}
	return moveMaintenance(s, ref, model.Scheduled, model.Cancelled, model.Operational, message, "", author, now)
}

func moveMaintenance(s *model.State, ref string, from, to model.Status, impact model.Impact,
	message, def, author string, now time.Time) (*Result, error) {
	cur, err := Find(s, ref)
	if err != nil {
		return nil, err
	}
	if cur.Kind != model.KindMaintenance {
		return nil, inputErrorf("%s is an incident; use the incident commands", cur.ID)
	}
	if st := cur.Latest().Status; st != from {
		return nil, inputErrorf("%s is %s; it must be %s to become %s", cur.ID, st, from, to)
	}
	msg, err := messageOrDefault(message, def)
	if err != nil {
		return nil, err
	}
	if err := checkAuthor(author); err != nil {
		return nil, err
	}
	in := cur.Clone()
	in.Updates = append(in.Updates, model.Update{
		At: nextStamp(cur, now), Status: to, Message: msg,
		Author: strings.TrimSpace(author), Impact: uniformImpact(in.Components, impact),
	})
	return finish(s, in)
}

// ParseTime accepts RFC 3339, or YYYY-MM-DDTHH:MM in the site timezone.
func ParseTime(v string, loc *time.Location) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", v, loc); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, inputErrorf("time %q must be RFC 3339 (2026-11-02T22:00:00Z) or YYYY-MM-DDTHH:MM in the site timezone", v)
}
