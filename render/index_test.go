package render

import (
	"strings"
	"testing"

	"github.com/Fab-IT-ApS/statusctl/model"
)

func TestIndexAllOperational(t *testing.T) {
	site := renderSite(t)
	idx := site["index.html"]
	contains(t, "index", idx,
		"All systems operational", "Public API", "Web portal", "Email", "<h3>Platform</h3>",
		"100.00% uptime", `href="./assets/style.css"`, `href="./assets/theme.css"`, `href="./assets/custom.css"`,
		`src="./assets/logo.svg"`, `href="./feed.atom"`, `href="https://example.com"`, "Last updated 23 Oct 2026 14:00 CEST")
	lacks(t, "index", idx, "FTP", "<script")
	if strings.Count(idx, `<li class="operational"`) != 3*90 {
		t.Errorf("want 90 uptime bars for each of 3 components")
	}
	contains(t, "theme.css", site["assets/theme.css"], "--primary: #123456")
	contains(t, "custom.css", site["assets/custom.css"], "letter-spacing")
	contains(t, "style.css", site["assets/style.css"], ".uptime")
	if _, ok := site["assets/logo.svg"]; !ok {
		t.Error("logo not copied")
	}
}

func TestIndexWithOpenIncident(t *testing.T) {
	site := renderSite(t, incident("2026-10-23-api-down", "API down",
		upd(d(23, 10), model.Investigating, "We are **on it**.", imp{"api": model.MajorOutage})))
	idx := site["index.html"]
	contains(t, "index", idx, `<section class="banner major-outage">`, "<h1>Major outage</h1>",
		`href="./incidents/2026-10-23-api-down/"`, "<strong>on it</strong>", "Investigating")
}

func TestIndexEscapesTitles(t *testing.T) {
	site := renderSite(t, incident("2026-10-23-xss", `<script>alert(1)</script>`,
		upd(d(23, 10), model.Investigating, "m", imp{"api": model.Degraded})))
	idx := site["index.html"]
	lacks(t, "index", idx, "<script>alert")
	contains(t, "index", idx, "&lt;script&gt;alert(1)&lt;/script&gt;")
}

func TestIndexUpcomingMaintenance(t *testing.T) {
	site := renderSite(t, maintenance("2026-10-23-db", "DB upgrade", d(25, 21), d(25, 23),
		upd(d(23, 9), model.Scheduled, "Planned.", imp{"web": model.Operational})))
	idx := site["index.html"]
	contains(t, "index", idx, "Upcoming maintenance", "DB upgrade", "Scheduled 25 Oct 2026 22:00 CET – 26 Oct 2026 00:00 CET",
		"All systems operational")
}

func TestIndexRecentlyResolved(t *testing.T) {
	site := renderSite(t,
		incident("2026-10-20-recent", "Recent blip",
			upd(d(20, 9), model.Investigating, "m", imp{"api": model.Degraded}),
			upd(d(20, 10), model.Resolved, "Fixed.", imp{"api": model.Operational})),
		incident("2026-10-01-old", "Old blip",
			upd(d(1, 9), model.Investigating, "m", imp{"api": model.Degraded}),
			upd(d(1, 10), model.Resolved, "Fixed.", imp{"api": model.Operational})))
	// The uptime bar tooltips legitimately name both events, so look only at
	// the "Past 7 days" section.
	_, recent, _ := strings.Cut(site["index.html"], "<h2>Past 7 days</h2>")
	contains(t, "index", recent, "Recent blip")
	lacks(t, "index", recent, "Old blip")
}

func TestIndexRetiredComponentIncident(t *testing.T) {
	site := renderSite(t, incident("2026-10-23-ftp", "FTP outage",
		upd(d(23, 10), model.Investigating, "m", imp{"ftp": model.MajorOutage})))
	idx := site["index.html"]
	contains(t, "index", idx, "All systems operational", "FTP outage")
}
