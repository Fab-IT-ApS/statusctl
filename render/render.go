// Package render turns repository state into a static site: HTML pages with
// no JavaScript, an Atom feed, a JSON API and Netlify _headers.
package render

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"path"
	"strings"
	"time"

	"statusctl/derive"
	"statusctl/markdown"
	"statusctl/model"
)

//go:embed templates/*.html static/style.css
var embedded embed.FS

const (
	uptimeDays   = 90
	recentWindow = 7 * 24 * time.Hour
	defaultColor = "#0b5fff"
)

var pageNames = []string{"index", "incident", "history", "month"}

type renderer struct {
	fsys     fs.FS
	s        *model.State
	now      time.Time
	statuses map[string]model.Impact
	base     *template.Template
	pages    map[string]*template.Template
	files    map[string][]byte
	logoPath string
}

// Render builds every file of the site, keyed by its published path.
func Render(fsys fs.FS, s *model.State, now time.Time) (map[string][]byte, error) {
	r := &renderer{fsys: fsys, s: s, now: now, statuses: derive.ComponentStatuses(s), files: map[string][]byte{}}
	if err := r.parseTemplates(); err != nil {
		return nil, err
	}
	for _, step := range r.steps() {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return r.files, nil
}

func (r *renderer) steps() []func() error {
	return []func() error{r.assets, r.index, r.incidentPages, r.history, r.feed, r.api, r.headers}
}

func (r *renderer) parseTemplates() error {
	loc := r.s.Config.Location()
	funcs := template.FuncMap{
		"fmtTime":     func(t time.Time) string { return t.In(loc).Format("2 Jan 2006 15:04 MST") },
		"fmtDate":     func(t time.Time) string { return t.In(loc).Format("2 Jan 2006") },
		"md":          markdown.Render,
		"impactClass": func(i model.Impact) string { return strings.ReplaceAll(string(i), "_", "-") },
	}
	base, err := template.New("base").Funcs(funcs).ParseFS(embedded, "templates/layout.html", "templates/partials.html")
	if err != nil {
		return err
	}
	r.base = base
	r.pages = map[string]*template.Template{}
	for _, name := range pageNames {
		clone, err := base.Clone()
		if err != nil {
			return err
		}
		if r.pages[name], err = clone.ParseFS(embedded, "templates/"+name+".html"); err != nil {
			return err
		}
	}
	return nil
}

type page struct {
	Site         model.Site
	Title        string
	Root         string
	LogoPath     string
	HasCustomCSS bool
	Updated      time.Time
	Body         any
}

func (r *renderer) page(out, tmpl, title, root string, body any) error {
	p := page{
		Site: r.s.Config.Site, Title: title, Root: root, LogoPath: r.logoPath,
		HasCustomCSS: r.s.Config.Site.CustomCSS != "", Updated: r.now, Body: body,
	}
	var buf bytes.Buffer
	if err := r.pages[tmpl].ExecuteTemplate(&buf, "layout", p); err != nil {
		return fmt.Errorf("rendering %s: %w", out, err)
	}
	r.files[out] = buf.Bytes()
	return nil
}

func (r *renderer) assets() error {
	css, err := embedded.ReadFile("static/style.css")
	if err != nil {
		return err
	}
	r.files["assets/style.css"] = css
	site := r.s.Config.Site
	color := site.PrimaryColor // validated as #rrggbb, safe to place in CSS
	if color == "" {
		color = defaultColor
	}
	r.files["assets/theme.css"] = []byte(":root { --primary: " + color + "; }\n")
	if site.Logo != "" {
		b, err := fs.ReadFile(r.fsys, site.Logo)
		if err != nil {
			return fmt.Errorf("site.logo: %w", err)
		}
		r.logoPath = "assets/logo" + strings.ToLower(path.Ext(site.Logo))
		r.files[r.logoPath] = b
	}
	if site.CustomCSS != "" {
		b, err := fs.ReadFile(r.fsys, site.CustomCSS)
		if err != nil {
			return fmt.Errorf("site.custom_css: %w", err)
		}
		r.files["assets/custom.css"] = b
	}
	return nil
}

type eventView struct {
	*model.Incident
	Root       string
	Latest     model.Update
	Impact     model.Impact
	Start, End time.Time // scheduled window; zero for incidents
}

func (r *renderer) event(in *model.Incident, root string) eventView {
	l := in.Latest()
	v := eventView{Incident: in, Root: root, Latest: l, Impact: model.Operational}
	for _, imp := range l.Impact {
		v.Impact = model.Worst(v.Impact, imp)
	}
	if in.Kind == model.KindMaintenance && l.Status == model.Scheduled {
		v.Impact = model.UnderMaintenance
	}
	if in.ScheduledStart != nil && in.ScheduledEnd != nil {
		v.Start, v.End = *in.ScheduledStart, *in.ScheduledEnd
	}
	return v
}

func (r *renderer) events(ins []*model.Incident, root string) []eventView {
	out := make([]eventView, 0, len(ins))
	for _, in := range ins {
		out = append(out, r.event(in, root))
	}
	return out
}

// when is the time an event is filed under: the scheduled start for
// maintenance, the first update for incidents.
func when(in *model.Incident) time.Time {
	if in.ScheduledStart != nil {
		return *in.ScheduledStart
	}
	return in.Started()
}

func newestFirst(us []model.Update) []model.Update {
	out := make([]model.Update, len(us))
	for i, u := range us {
		out[len(us)-1-i] = u
	}
	return out
}

type bannerView struct {
	Impact model.Impact
	Text   string
}

var bannerText = map[model.Impact]string{
	model.Operational:      "All systems operational",
	model.UnderMaintenance: "Maintenance in progress",
	model.Degraded:         "Degraded performance",
	model.PartialOutage:    "Partial outage",
	model.MajorOutage:      "Major outage",
}

type componentView struct {
	model.Component
	Status model.Impact
	Uptime derive.Uptime
}

type groupView struct {
	Name       string
	Components []componentView
}

type indexView struct {
	Banner   bannerView
	Active   []eventView
	Upcoming []eventView
	Groups   []groupView
	Recent   []eventView
}

func (r *renderer) index() error {
	overall := derive.Overall(r.s)
	body := indexView{
		Banner:   bannerView{Impact: overall, Text: bannerText[overall]},
		Active:   r.events(derive.ActiveEvents(r.s), "./"),
		Upcoming: r.events(derive.Upcoming(r.s), "./"),
		Groups:   r.groups(),
		Recent:   r.events(derive.RecentlyResolved(r.s, r.now, recentWindow), "./"),
	}
	return r.page("index.html", "index", "", "./", body)
}

// groups keeps status.yaml order; a group appears where its first component does.
func (r *renderer) groups() []groupView {
	var out []groupView
	pos := map[string]int{}
	for _, c := range r.s.Config.ActiveComponents() {
		i, ok := pos[c.Group]
		if !ok {
			i = len(out)
			pos[c.Group] = i
			out = append(out, groupView{Name: c.Group})
		}
		out[i].Components = append(out[i].Components, componentView{
			Component: c,
			Status:    r.statuses[c.ID],
			Uptime:    derive.ComponentUptime(r.s, c.ID, r.now, uptimeDays),
		})
	}
	return out
}
