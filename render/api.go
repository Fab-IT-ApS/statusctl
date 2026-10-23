package render

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"statusctl/derive"
	"statusctl/model"
)

const apiWindow = 90 * 24 * time.Hour

const headers = `/*
  Content-Security-Policy: default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'
  X-Content-Type-Options: nosniff
  Referrer-Policy: no-referrer
/api/*
  Access-Control-Allow-Origin: *
/feed.atom
  Access-Control-Allow-Origin: *
  Content-Type: application/atom+xml; charset=utf-8
`

type apiPage struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

type apiComponent struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Group  string       `json:"group,omitempty"`
	Status model.Impact `json:"status"`
}

type apiEvent struct {
	ID             string         `json:"id"`
	Kind           model.Kind     `json:"kind"`
	Title          string         `json:"title"`
	Status         model.Status   `json:"status"`
	URL            string         `json:"url"`
	Components     []string       `json:"components"`
	StartedAt      time.Time      `json:"started_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	ScheduledStart *time.Time     `json:"scheduled_start,omitempty"`
	ScheduledEnd   *time.Time     `json:"scheduled_end,omitempty"`
	Updates        []model.Update `json:"updates,omitempty"`
}

// absURL joins the site URL and a path without doubling the slash.
func (r *renderer) absURL(p string) string {
	return strings.TrimRight(r.s.Config.Site.URL, "/") + "/" + p
}

func (r *renderer) apiPage() apiPage {
	return apiPage{Name: r.s.Config.Site.Name, URL: r.absURL(""), UpdatedAt: r.now.UTC()}
}

func (r *renderer) apiEvent(in *model.Incident, withUpdates bool) apiEvent {
	e := apiEvent{
		ID: in.ID, Kind: in.Kind, Title: in.Title, Status: in.Latest().Status,
		URL: r.absURL("incidents/" + in.ID + "/"), Components: in.Components,
		StartedAt: in.Started(), UpdatedAt: in.LastUpdated(),
		ScheduledStart: in.ScheduledStart, ScheduledEnd: in.ScheduledEnd,
	}
	if withUpdates {
		e.Updates = in.Updates
	}
	return e
}

func (r *renderer) apiEvents(ins []*model.Incident, withUpdates bool) []apiEvent {
	out := make([]apiEvent, 0, len(ins))
	for _, in := range ins {
		out = append(out, r.apiEvent(in, withUpdates))
	}
	return out
}

func (r *renderer) api() error {
	comps := make([]apiComponent, 0)
	for _, c := range r.s.Config.ActiveComponents() {
		comps = append(comps, apiComponent{ID: c.ID, Name: c.Name, Group: c.Group, Status: r.statuses[c.ID]})
	}
	status := struct {
		Page       apiPage        `json:"page"`
		Status     model.Impact   `json:"status"`
		Components []apiComponent `json:"components"`
		Active     []apiEvent     `json:"active"`
		Upcoming   []apiEvent     `json:"upcoming"`
	}{r.apiPage(), derive.Overall(r.s), comps,
		r.apiEvents(derive.ActiveEvents(r.s), false), r.apiEvents(derive.Upcoming(r.s), false)}
	if err := r.writeJSON("api/v1/status.json", status); err != nil {
		return err
	}

	cutoff := r.now.Add(-apiWindow)
	var recent []*model.Incident
	for _, in := range r.s.Incidents {
		if !in.LastUpdated().Before(cutoff) {
			recent = append(recent, in)
		}
	}
	slices.SortStableFunc(recent, func(a, b *model.Incident) int { return when(b).Compare(when(a)) })
	incidents := struct {
		Page      apiPage    `json:"page"`
		Incidents []apiEvent `json:"incidents"`
	}{r.apiPage(), r.apiEvents(recent, true)}
	return r.writeJSON("api/v1/incidents.json", incidents)
}

func (r *renderer) writeJSON(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	r.files[p] = append(b, '\n')
	return nil
}

func (r *renderer) headers() error {
	r.files["_headers"] = []byte(headers)
	return nil
}
