package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	IncidentDir = "incidents"
	MaxTitle    = 200
	MaxMessage  = 10000
)

var incidentIDRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-[a-z0-9]+(-[a-z0-9]+)*$`)

// Update is one entry in an event's timeline. Impact is a full snapshot for
// every component the event lists, not a delta.
type Update struct {
	At      time.Time         `json:"at"`
	Status  Status            `json:"status"`
	Message string            `json:"message"`
	Author  string            `json:"author"`
	Impact  map[string]Impact `json:"impact"`
}

// Incident is an incident or a maintenance window, stored as incidents/<id>.json.
type Incident struct {
	ID             string     `json:"id"`
	Kind           Kind       `json:"kind"`
	Title          string     `json:"title"`
	Components     []string   `json:"components"`
	ScheduledStart *time.Time `json:"scheduled_start,omitempty"`
	ScheduledEnd   *time.Time `json:"scheduled_end,omitempty"`
	Updates        []Update   `json:"updates"`
}

// ParseIncident decodes an incident file. Unknown fields are errors.
func ParseIncident(data []byte) (*Incident, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var in Incident
	if err := dec.Decode(&in); err != nil {
		return nil, err
	}
	return &in, nil
}

func (in *Incident) Marshal() ([]byte, error) {
	b, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (in *Incident) Latest() Update         { return in.Updates[len(in.Updates)-1] }
func (in *Incident) Started() time.Time     { return in.Updates[0].At }
func (in *Incident) LastUpdated() time.Time { return in.Latest().At }
func (in *Incident) Path() string           { return IncidentDir + "/" + in.ID + ".json" }

// Clone returns a deep copy, so operations never mutate loaded state.
func (in *Incident) Clone() *Incident {
	out := *in
	out.Components = slices.Clone(in.Components)
	out.Updates = make([]Update, len(in.Updates))
	for i, u := range in.Updates {
		u.Impact = maps.Clone(u.Impact)
		out.Updates[i] = u
	}
	if in.ScheduledStart != nil {
		t := *in.ScheduledStart
		out.ScheduledStart = &t
	}
	if in.ScheduledEnd != nil {
		t := *in.ScheduledEnd
		out.ScheduledEnd = &t
	}
	return &out
}

// ValidateIncident checks an event against this config.
func (c *Config) ValidateIncident(in *Incident) error {
	var p problems
	c.validateIncident(in, in.Path(), &p)
	return p.err()
}

func (c *Config) validateIncident(in *Incident, file string, p *problems) {
	if !incidentIDRe.MatchString(in.ID) {
		p.addf("%s: id %q must look like 2026-10-23-short-title", file, in.ID)
	} else if path.Base(file) != in.ID+".json" {
		p.addf("%s: id %q does not match the file name", file, in.ID)
	}
	if in.Kind != KindIncident && in.Kind != KindMaintenance {
		p.addf("%s: kind %q must be incident or maintenance", file, in.Kind)
	}
	if strings.TrimSpace(in.Title) == "" || strings.ContainsAny(in.Title, "\r\n") || utf8.RuneCountInString(in.Title) > MaxTitle {
		p.addf("%s: title must be one line of 1-%d characters", file, MaxTitle)
	}
	if len(in.Components) == 0 {
		p.addf("%s: components must list at least one component", file)
	}
	listed := map[string]bool{}
	for _, id := range in.Components {
		if listed[id] {
			p.addf("%s: component %q is listed twice", file, id)
		}
		listed[id] = true
		if _, ok := c.Component(id); !ok {
			p.addf("%s: component %q is not in status.yaml (mark it retired: true instead of deleting it)", file, id)
		}
	}
	switch in.Kind {
	case KindMaintenance:
		if in.ScheduledStart == nil || in.ScheduledEnd == nil {
			p.addf("%s: maintenance needs scheduled_start and scheduled_end", file)
		} else if !in.ScheduledEnd.After(*in.ScheduledStart) {
			p.addf("%s: scheduled_end must be after scheduled_start", file)
		}
	case KindIncident:
		if in.ScheduledStart != nil || in.ScheduledEnd != nil {
			p.addf("%s: only maintenance has a schedule", file)
		}
	}
	if len(in.Updates) == 0 {
		p.addf("%s: updates must not be empty", file)
		return
	}
	for i, u := range in.Updates {
		where := fmt.Sprintf("%s: updates[%d]", file, i)
		if u.At.IsZero() {
			p.addf("%s.at is required", where)
		} else if i > 0 && u.At.Before(in.Updates[i-1].At) {
			p.addf("%s.at is earlier than the update before it", where)
		}
		if !u.Status.ValidFor(in.Kind) {
			p.addf("%s.status %q is not valid for %s", where, u.Status, in.Kind)
		}
		if strings.TrimSpace(u.Message) == "" || utf8.RuneCountInString(u.Message) > MaxMessage {
			p.addf("%s.message must be 1-%d characters", where, MaxMessage)
		}
		if strings.TrimSpace(u.Author) == "" {
			p.addf("%s.author is required", where)
		}
		if len(u.Impact) != len(listed) {
			p.addf("%s.impact must give an impact for exactly the listed components", where)
		}
		for _, id := range slices.Sorted(maps.Keys(u.Impact)) {
			if !listed[id] {
				p.addf("%s.impact names %q, which is not in components", where, id)
			}
			if !u.Impact[id].Valid() {
				p.addf("%s.impact[%q] %q is not a valid impact", where, id, u.Impact[id])
			}
		}
	}
}
