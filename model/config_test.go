package model

import (
	"errors"
	"strings"
	"testing"
)

const validConfig = `site:
  name: "Acme Status"
  url: "https://status.acme.example"
  timezone: Europe/Copenhagen
  logo: assets/logo.svg
  primary_color: "#0b5fff"
  footer_links:
    - label: "Acme"
      url: "https://acme.example"
components:
  - { id: api, name: Public API, group: Platform }
  - { id: web, name: Web portal, group: Platform }
  - { id: ftp, name: FTP, retired: true }
`

func TestParseConfigValid(t *testing.T) {
	c, err := ParseConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	if c.Site.Name != "Acme Status" || c.Location().String() != "Europe/Copenhagen" {
		t.Errorf("site parsed wrong: %+v", c.Site)
	}
	if got := len(c.ActiveComponents()); got != 2 {
		t.Errorf("ActiveComponents() = %d, want 2", got)
	}
	ftp, ok := c.Component("ftp")
	if !ok || !ftp.Retired {
		t.Error("Component(ftp) should exist and be retired")
	}
	if _, ok := c.Component("nope"); ok {
		t.Error("Component(nope) should not exist")
	}
}

func TestParseConfigRejects(t *testing.T) {
	cases := []struct{ name, from, to, want string }{
		{"missing name", `name: "Acme Status"`, `name: ""`, "site.name is required"},
		{"bad url", `"https://status.acme.example"`, `"status.acme.example"`, "site.url"},
		{"bad timezone", "Europe/Copenhagen", "Mars/Olympus", "not a known IANA timezone"},
		{"css injection in color", `"#0b5fff"`, `"red; } body { x"`, "primary_color"},
		{"logo escapes repo", "assets/logo.svg", "../etc/passwd.svg", "site.logo"},
		{"logo wrong type", "assets/logo.svg", "assets/logo.html", "site.logo"},
		{"footer javascript url", `"https://acme.example"`, `"javascript:alert(1)"`, "footer_links[0].url"},
		{"bad component id", "id: api,", "id: API,", "components[0].id"},
		{"duplicate component id", "id: web,", "id: api,", "duplicated"},
		{"unknown field", "timezone:", "timezoen:", "field timezoen not found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := strings.Replace(validConfig, c.from, c.to, 1)
			if src == validConfig {
				t.Fatalf("replacement %q did not apply", c.from)
			}
			_, err := ParseConfig([]byte(src))
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want *ValidationError, got %v", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

func TestSafeURL(t *testing.T) {
	for u, want := range map[string]bool{
		"https://example.com":     true,
		"http://example.com/x":    true,
		"mailto:ops@example.com":  true,
		"javascript:alert(1)":     false,
		"/relative":               false,
		"https://":                false,
		"data:text/html,<b>x</b>": false,
	} {
		if got := SafeURL(u); got != want {
			t.Errorf("SafeURL(%q) = %v, want %v", u, got, want)
		}
	}
}
