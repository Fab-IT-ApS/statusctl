package render

import (
	"bytes"
	"encoding/xml"
	"slices"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

const feedSize = 50

type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Author  atomPerson  `xml:"author"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomPerson struct {
	Name string `xml:"name"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type atomEntry struct {
	Title     string      `xml:"title"`
	ID        string      `xml:"id"`
	Updated   string      `xml:"updated"`
	Published string      `xml:"published"`
	Link      atomLink    `xml:"link"`
	Content   atomContent `xml:"content"`
}

type atomContent struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

func (r *renderer) feed() error {
	events := slices.Clone(r.s.Incidents)
	slices.SortStableFunc(events, func(a, b *model.Incident) int { return b.LastUpdated().Compare(a.LastUpdated()) })
	if len(events) > feedSize {
		events = events[:feedSize]
	}
	updated := r.now.UTC()
	if len(events) > 0 {
		updated = events[0].LastUpdated().UTC()
	}
	site := r.s.Config.Site
	f := atomFeed{
		Title: site.Name, ID: r.absURL(""), Updated: updated.Format(time.RFC3339),
		Author: atomPerson{Name: site.Name},
		Links: []atomLink{
			{Href: r.absURL(""), Rel: "alternate", Type: "text/html"},
			{Href: r.absURL("feed.atom"), Rel: "self", Type: "application/atom+xml"},
		},
	}
	for _, in := range events {
		var body bytes.Buffer
		if err := r.base.ExecuteTemplate(&body, "feed-entry", newestFirst(in.Updates)); err != nil {
			return err
		}
		u := r.absURL("incidents/" + in.ID + "/")
		f.Entries = append(f.Entries, atomEntry{
			Title:     in.Title + " (" + in.Latest().Status.Label() + ")",
			ID:        u,
			Updated:   in.LastUpdated().UTC().Format(time.RFC3339),
			Published: in.Started().UTC().Format(time.RFC3339),
			Link:      atomLink{Href: u, Rel: "alternate", Type: "text/html"},
			Content:   atomContent{Type: "html", Body: body.String()},
		})
	}
	b, err := xml.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	r.files["feed.atom"] = append(append([]byte(xml.Header), b...), '\n')
	return nil
}
