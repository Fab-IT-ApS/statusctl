package ops

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

type NewIncidentArgs struct {
	Title   string
	Status  model.Status // defaults to investigating
	Impacts map[string]model.Impact
	Message string
	Author  string
}

func NewIncident(s *model.State, a NewIncidentArgs, now time.Time) (*Result, error) {
	title, err := checkTitle(a.Title)
	if err != nil {
		return nil, err
	}
	status := a.Status
	if status == "" {
		status = model.Investigating
	}
	if !status.ValidFor(model.KindIncident) || status.Terminal() {
		return nil, inputErrorf("a new incident's status must be investigating, identified or monitoring, not %q", status)
	}
	if len(a.Impacts) == 0 {
		return nil, inputErrorf("name at least one affected component with -c ID=IMPACT")
	}
	set := map[string]bool{}
	for _, id := range slices.Sorted(maps.Keys(a.Impacts)) {
		if err := checkComponent(s.Config, id); err != nil {
			return nil, err
		}
		if err := checkIncidentImpact(id, a.Impacts[id]); err != nil {
			return nil, err
		}
		set[id] = true
	}
	msg, err := checkMessage(a.Message)
	if err != nil {
		return nil, err
	}
	if err := checkAuthor(a.Author); err != nil {
		return nil, err
	}
	in := &model.Incident{
		ID:         NewID(s, title, now),
		Kind:       model.KindIncident,
		Title:      title,
		Components: orderedComponents(s.Config, set),
		Updates: []model.Update{{
			At: stamp(now), Status: status, Message: msg,
			Author: strings.TrimSpace(a.Author), Impact: maps.Clone(a.Impacts),
		}},
	}
	return finish(s, in)
}

type UpdateArgs struct {
	Ref     string
	Status  model.Status // empty keeps the current status
	Impacts map[string]model.Impact
	Message string
	Author  string
	Reopen  bool
}

func UpdateIncident(s *model.State, a UpdateArgs, now time.Time) (*Result, error) {
	cur, err := Find(s, a.Ref)
	if err != nil {
		return nil, err
	}
	if cur.Kind != model.KindIncident {
		return nil, inputErrorf("%s is a maintenance window; use the maintenance commands", cur.ID)
	}
	last := cur.Latest()
	if last.Status.Terminal() && !a.Reopen {
		return nil, inputErrorf("%s is resolved; pass --reopen to post to it again", cur.ID)
	}
	status := a.Status
	switch {
	case status != "":
	case last.Status.Terminal():
		status = model.Investigating
	default:
		status = last.Status
	}
	if !status.ValidFor(model.KindIncident) {
		return nil, inputErrorf("status %q must be investigating, identified, monitoring or resolved", status)
	}
	if last.Status.Terminal() && status.Terminal() {
		return nil, inputErrorf("reopening %s needs a --status other than resolved", cur.ID)
	}
	impact := maps.Clone(last.Impact)
	if status == model.Resolved {
		if len(a.Impacts) > 0 {
			return nil, inputErrorf("resolving clears every impact; drop the -c flags")
		}
		for id := range impact {
			impact[id] = model.Operational
		}
	}
	for _, id := range slices.Sorted(maps.Keys(a.Impacts)) {
		if !slices.Contains(cur.Components, id) {
			return nil, inputErrorf("component %q is not part of %s; open a new incident for it", id, cur.ID)
		}
		if err := checkIncidentImpact(id, a.Impacts[id]); err != nil {
			return nil, err
		}
		impact[id] = a.Impacts[id]
	}
	msg, err := checkMessage(a.Message)
	if err != nil {
		return nil, err
	}
	if err := checkAuthor(a.Author); err != nil {
		return nil, err
	}
	in := cur.Clone()
	in.Updates = append(in.Updates, model.Update{
		At: nextStamp(cur, now), Status: status, Message: msg,
		Author: strings.TrimSpace(a.Author), Impact: impact,
	})
	return finish(s, in)
}

func ResolveIncident(s *model.State, ref, message, author string, now time.Time) (*Result, error) {
	return UpdateIncident(s, UpdateArgs{Ref: ref, Status: model.Resolved, Message: message, Author: author}, now)
}
