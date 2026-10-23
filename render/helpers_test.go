package render

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"statusctl/model"
)

const testConfig = `site:
  name: Test Status
  url: "https://status.example.com"
  timezone: Europe/Copenhagen
  logo: assets/logo.svg
  primary_color: "#123456"
  custom_css: custom.css
  footer_links:
    - label: Example
      url: "https://example.com"
components:
  - { id: api, name: Public API, group: Platform }
  - { id: web, name: Web portal, group: Platform }
  - { id: mail, name: Email }
  - { id: ftp, name: FTP, retired: true }
`

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

type imp = map[string]model.Impact

func upd(at time.Time, st model.Status, msg string, impact imp) model.Update {
	return model.Update{At: at, Status: st, Message: msg, Author: "Ops", Impact: impact}
}

func incident(id, title string, updates ...model.Update) *model.Incident {
	return &model.Incident{ID: id, Kind: model.KindIncident, Title: title, Updates: updates,
		Components: slices.Sorted(maps.Keys(updates[0].Impact))}
}

func maintenance(id, title string, start, end time.Time, updates ...model.Update) *model.Incident {
	in := incident(id, title, updates...)
	in.Kind = model.KindMaintenance
	in.ScheduledStart, in.ScheduledEnd = &start, &end
	return in
}

func fixture(t *testing.T, config string, incidents ...*model.Incident) (fstest.MapFS, *model.State) {
	t.Helper()
	fsys := fstest.MapFS{
		"status.yaml":     {Data: []byte(config)},
		"assets/logo.svg": {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)},
		"custom.css":      {Data: []byte("body { letter-spacing: 0; }\n")},
	}
	for _, in := range incidents {
		b, err := in.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		fsys[in.Path()] = &fstest.MapFile{Data: b}
	}
	s, err := model.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return fsys, s
}

func renderSite(t *testing.T, incidents ...*model.Incident) map[string]string {
	t.Helper()
	return renderWith(t, testConfig, incidents...)
}

func renderWith(t *testing.T, config string, incidents ...*model.Incident) map[string]string {
	t.Helper()
	fsys, s := fixture(t, config, incidents...)
	files, err := Render(fsys, s, now)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for k, v := range files {
		out[k] = string(v)
	}
	return out
}

func contains(t *testing.T, page, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("%s: missing %q", page, w)
		}
	}
}

func lacks(t *testing.T, page, body string, denies ...string) {
	t.Helper()
	for _, d := range denies {
		if strings.Contains(body, d) {
			t.Errorf("%s: must not contain %q", page, d)
		}
	}
}

func d(day, hour int) time.Time { return time.Date(2026, 10, day, hour, 0, 0, 0, time.UTC) }
