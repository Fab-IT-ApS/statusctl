// Package ops implements every status change as a pure function from
// repository state to the files that must be written.
package ops

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"statusctl/model"
)

// InputError means the caller asked for something invalid. It is never retried.
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

func inputErrorf(format string, args ...any) error {
	return &InputError{Msg: fmt.Sprintf(format, args...)}
}

// Result is the event an operation produced and the files to write for it.
type Result struct {
	Incident *model.Incident
	URL      string
	Files    map[string][]byte
}

func (r *Result) CommitMessage() string {
	return fmt.Sprintf("%s %s: %s", r.Incident.Kind, r.Incident.ID, r.Incident.Latest().Status)
}

// Find resolves an exact ID or a unique ID prefix.
func Find(s *model.State, ref string) (*model.Incident, error) {
	if ref == "" {
		return nil, inputErrorf("an incident or maintenance ID is required")
	}
	var matches []*model.Incident
	for _, in := range s.Incidents {
		if in.ID == ref {
			return in, nil
		}
		if strings.HasPrefix(in.ID, ref) {
			matches = append(matches, in)
		}
	}
	switch len(matches) {
	case 0:
		return nil, inputErrorf("no incident or maintenance matches %q", ref)
	case 1:
		return matches[0], nil
	}
	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = m.ID
	}
	return nil, inputErrorf("%q is ambiguous: %s", ref, strings.Join(ids, ", "))
}

// NewID builds YYYY-MM-DD-<slug> from the UTC date, adding -2, -3 … on collision.
func NewID(s *model.State, title string, now time.Time) string {
	base := now.UTC().Format("2006-01-02") + "-" + slug(title)
	id := base
	for n := 2; s.Incident(id) != nil; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

var danish = strings.NewReplacer("æ", "ae", "ø", "oe", "å", "aa")

func slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range danish.Replace(strings.ToLower(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if dash {
				b.WriteByte('-')
				dash = false
			}
			b.WriteRune(r)
		case b.Len() > 0:
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	if b.Len() == 0 {
		return "event"
	}
	return b.String()
}

func checkTitle(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" || strings.ContainsAny(t, "\r\n") || utf8.RuneCountInString(t) > model.MaxTitle {
		return "", inputErrorf("the title must be one line of 1-%d characters", model.MaxTitle)
	}
	return t, nil
}

func checkMessage(m string) (string, error) {
	m = strings.TrimSpace(m)
	if m == "" {
		return "", inputErrorf("a message is required")
	}
	if utf8.RuneCountInString(m) > model.MaxMessage {
		return "", inputErrorf("the message is longer than %d characters", model.MaxMessage)
	}
	return m, nil
}

func messageOrDefault(m, def string) (string, error) {
	if strings.TrimSpace(m) == "" && def != "" {
		return def, nil
	}
	return checkMessage(m)
}

func checkAuthor(a string) error {
	if strings.TrimSpace(a) == "" {
		return inputErrorf("an author is required")
	}
	return nil
}

func checkComponent(cfg *model.Config, id string) error {
	c, ok := cfg.Component(id)
	if !ok {
		var known []string
		for _, a := range cfg.ActiveComponents() {
			known = append(known, a.ID)
		}
		return inputErrorf("unknown component %q (known: %s)", id, strings.Join(known, ", "))
	}
	if c.Retired {
		return inputErrorf("component %q is retired", id)
	}
	return nil
}

func checkIncidentImpact(id string, imp model.Impact) error {
	if !imp.Valid() || imp == model.UnderMaintenance {
		return inputErrorf("impact %q for %s must be one of operational, degraded, partial_outage, major_outage", imp, id)
	}
	return nil
}

// orderedComponents returns the IDs in set, in status.yaml order.
func orderedComponents(cfg *model.Config, set map[string]bool) []string {
	var out []string
	for _, c := range cfg.Components {
		if set[c.ID] {
			out = append(out, c.ID)
		}
	}
	return out
}

func uniformImpact(ids []string, imp model.Impact) map[string]model.Impact {
	out := make(map[string]model.Impact, len(ids))
	for _, id := range ids {
		out[id] = imp
	}
	return out
}

func stamp(now time.Time) time.Time { return now.UTC().Truncate(time.Second) }

// nextStamp never goes backwards, even when the poster's clock is behind.
func nextStamp(cur *model.Incident, now time.Time) time.Time {
	t := stamp(now)
	if last := cur.LastUpdated(); t.Before(last) {
		return last
	}
	return t
}

func finish(s *model.State, in *model.Incident) (*Result, error) {
	if err := s.Config.ValidateIncident(in); err != nil {
		return nil, err
	}
	b, err := in.Marshal()
	if err != nil {
		return nil, err
	}
	return &Result{
		Incident: in,
		URL:      strings.TrimRight(s.Config.Site.URL, "/") + "/incidents/" + in.ID + "/",
		Files:    map[string][]byte{in.Path(): b},
	}, nil
}
