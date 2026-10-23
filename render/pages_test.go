package render

import (
	"strings"
	"testing"
	"time"

	"statusctl/model"
)

func TestIncidentPage(t *testing.T) {
	site := renderSite(t, incident("2026-10-20-api-down", "API down",
		upd(d(20, 9), model.Investigating, "First message.", imp{"api": model.MajorOutage, "ftp": model.MajorOutage}),
		upd(d(20, 10), model.Resolved, "Second message.", imp{"api": model.Operational, "ftp": model.Operational})))
	pg, ok := site["incidents/2026-10-20-api-down/index.html"]
	if !ok {
		t.Fatal("incident page missing")
	}
	contains(t, "incident", pg, "<title>API down · Test Status</title>", "<h1>API down</h1>",
		"Affects: Public API, FTP", `href="../../assets/style.css"`, "Resolved", "Investigating")
	if strings.Index(pg, "Second message.") > strings.Index(pg, "First message.") {
		t.Error("timeline must be newest first")
	}
}

func TestMaintenancePage(t *testing.T) {
	site := renderSite(t, maintenance("2026-10-23-db", "DB upgrade", d(28, 21), d(28, 23),
		upd(d(23, 9), model.Scheduled, "Planned.", imp{"web": model.Operational})))
	pg := site["incidents/2026-10-23-db/index.html"]
	contains(t, "maintenance", pg, `<p class="kind">Maintenance</p>`, "Scheduled 28 Oct 2026 22:00 CET – 29 Oct 2026 00:00 CET")
}

func TestHistory(t *testing.T) {
	site := renderSite(t,
		incident("2026-09-30-late", "Late September",
			upd(time.Date(2026, 9, 30, 21, 0, 0, 0, time.UTC), model.Investigating, "m", imp{"api": model.Degraded})),
		incident("2026-10-20-oct", "October one",
			upd(d(20, 9), model.Investigating, "m", imp{"api": model.Degraded})))
	idx := site["history/index.html"]
	contains(t, "history", idx, `href="../history/2026-10/"`, "October 2026", `href="../history/2026-09/"`, "September 2026")
	if strings.Index(idx, "October 2026") > strings.Index(idx, "September 2026") {
		t.Error("months must be newest first")
	}
	oct := site["history/2026-10/index.html"]
	contains(t, "month", oct, "<h1>October 2026</h1>", "October one", `href="../../incidents/2026-10-20-oct/"`)
	lacks(t, "month", oct, "Late September")
}

func TestHistoryEmpty(t *testing.T) {
	site := renderSite(t)
	contains(t, "history", site["history/index.html"], "No incidents or maintenance recorded yet.")
}
