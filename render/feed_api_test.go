package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
	"time"

	"statusctl/model"
)

func TestStatusJSON(t *testing.T) {
	site := renderSite(t,
		incident("2026-10-23-api", "API slow", upd(d(23, 10), model.Identified, "m", imp{"api": model.Degraded})),
		maintenance("2026-10-23-db", "DB", d(28, 21), d(28, 23), upd(d(23, 9), model.Scheduled, "m", imp{"web": model.Operational})))
	var got struct {
		Page struct {
			Name, URL string
			UpdatedAt time.Time `json:"updated_at"`
		}
		Status     model.Impact
		Components []struct {
			ID, Name, Group string
			Status          model.Impact
		}
		Active []struct {
			ID, URL string
			Status  model.Status
		}
		Upcoming []struct {
			ID             string
			ScheduledStart *time.Time `json:"scheduled_start"`
		}
	}
	if err := json.Unmarshal([]byte(site["api/v1/status.json"]), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != model.Degraded || got.Page.Name != "Test Status" || !got.Page.UpdatedAt.Equal(now) {
		t.Errorf("page/status wrong: %+v", got)
	}
	if len(got.Components) != 3 || got.Components[0].ID != "api" || got.Components[0].Status != model.Degraded || got.Components[0].Group != "Platform" {
		t.Errorf("components wrong: %+v", got.Components)
	}
	if len(got.Active) != 1 || got.Active[0].URL != "https://status.example.com/incidents/2026-10-23-api/" || got.Active[0].Status != model.Identified {
		t.Errorf("active wrong: %+v", got.Active)
	}
	if len(got.Upcoming) != 1 || got.Upcoming[0].ScheduledStart == nil {
		t.Errorf("upcoming wrong: %+v", got.Upcoming)
	}
}

func TestStatusJSONEmptyListsAreArrays(t *testing.T) {
	body := renderSite(t)["api/v1/status.json"]
	contains(t, "status.json", body, `"active": []`, `"upcoming": []`)
	contains(t, "incidents.json", renderSite(t)["api/v1/incidents.json"], `"incidents": []`)
}

func TestIncidentsJSONWindow(t *testing.T) {
	site := renderSite(t,
		incident("2026-07-01-old", "Old", upd(time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), model.Investigating, "m", imp{"api": model.Degraded}),
			upd(time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC), model.Resolved, "m", imp{"api": model.Operational})),
		incident("2026-10-13-new", "New", upd(d(13, 9), model.Investigating, "first", imp{"api": model.Degraded})))
	var got struct {
		Incidents []struct {
			ID      string
			Updates []model.Update
		}
	}
	if err := json.Unmarshal([]byte(site["api/v1/incidents.json"]), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Incidents) != 1 || got.Incidents[0].ID != "2026-10-13-new" || got.Incidents[0].Updates[0].Message != "first" {
		t.Errorf("incidents.json = %+v", got)
	}
}

func TestTrailingSlashSiteURL(t *testing.T) {
	cfg := strings.Replace(testConfig, `"https://status.example.com"`, `"https://status.example.com/"`, 1)
	site := renderWith(t, cfg, incident("2026-10-23-api", "API", upd(d(23, 10), model.Investigating, "m", imp{"api": model.Degraded})))
	for _, f := range []string{"feed.atom", "api/v1/status.json", "api/v1/incidents.json"} {
		lacks(t, f, site[f], "example.com//")
		contains(t, f, site[f], "https://status.example.com/incidents/2026-10-23-api/")
	}
}

type testFeed struct {
	Title   string `xml:"title"`
	Entries []struct {
		Title   string `xml:"title"`
		ID      string `xml:"id"`
		Content string `xml:"content"`
	} `xml:"entry"`
}

func TestFeed(t *testing.T) {
	site := renderSite(t, incident("2026-10-23-api", "API slow",
		upd(d(23, 9), model.Investigating, "Looking <b>now</b>.", imp{"api": model.Degraded}),
		upd(d(23, 10), model.Resolved, "**Fixed**.", imp{"api": model.Operational})))
	raw := site["feed.atom"]
	if !strings.HasPrefix(raw, "<?xml") || !strings.Contains(raw, `xmlns="http://www.w3.org/2005/Atom"`) {
		t.Fatalf("not an Atom document: %.200s", raw)
	}
	var f testFeed
	if err := xml.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatal(err)
	}
	if f.Title != "Test Status" || len(f.Entries) != 1 {
		t.Fatalf("feed = %+v", f)
	}
	e := f.Entries[0]
	if e.Title != "API slow (Resolved)" || e.ID != "https://status.example.com/incidents/2026-10-23-api/" {
		t.Errorf("entry = %+v", e)
	}
	if !strings.Contains(e.Content, "<strong>Fixed</strong>") || strings.Contains(e.Content, "<b>now</b>") {
		t.Errorf("content must be sanitised HTML: %q", e.Content)
	}
	if strings.Index(e.Content, "Fixed") > strings.Index(e.Content, "Looking") {
		t.Error("entry content must be newest first")
	}
}

func TestFeedCappedAt50(t *testing.T) {
	var ins []*model.Incident
	for i := 0; i < 55; i++ {
		ins = append(ins, incident(fmt.Sprintf("2026-10-20-e%d", i), fmt.Sprintf("E%d", i),
			upd(d(20, 0).Add(time.Duration(i)*time.Minute), model.Investigating, "m", imp{"api": model.Degraded})))
	}
	var f testFeed
	if err := xml.Unmarshal([]byte(renderSite(t, ins...)["feed.atom"]), &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 50 || f.Entries[0].Title != "E54 (Investigating)" {
		t.Errorf("want the 50 most recently updated, newest first; got %d, first %q", len(f.Entries), f.Entries[0].Title)
	}
}

func TestHeaders(t *testing.T) {
	h := renderSite(t)["_headers"]
	contains(t, "_headers", h,
		"Content-Security-Policy: default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
		"X-Content-Type-Options: nosniff", "Referrer-Policy: no-referrer", "/api/*\n  Access-Control-Allow-Origin: *")
}

func TestNoScriptsAnywhere(t *testing.T) {
	site := renderSite(t, incident("2026-10-23-x", "<script>x</script>",
		upd(d(23, 9), model.Investigating, "<script>alert(1)</script>", imp{"api": model.Degraded})))
	for name, body := range site {
		if strings.HasSuffix(name, ".html") || name == "feed.atom" {
			lacks(t, name, body, "<script")
		}
	}
}
