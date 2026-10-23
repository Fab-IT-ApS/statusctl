package model

import (
	"bytes"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigFile is the hand-edited site configuration at the repository root.
const ConfigFile = "status.yaml"

type Config struct {
	Site       Site        `yaml:"site"`
	Components []Component `yaml:"components"`

	loc *time.Location
}

type Site struct {
	Name         string `yaml:"name"`
	URL          string `yaml:"url"`
	Timezone     string `yaml:"timezone"`
	Logo         string `yaml:"logo"`
	PrimaryColor string `yaml:"primary_color"`
	CustomCSS    string `yaml:"custom_css"`
	FooterLinks  []Link `yaml:"footer_links"`
}

type Link struct {
	Label string `yaml:"label"`
	URL   string `yaml:"url"`
}

type Component struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Group       string `yaml:"group"`
	Description string `yaml:"description"`
	Retired     bool   `yaml:"retired"`
}

var (
	componentIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	colorRe       = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	assetPathRe   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./-]*$`)
	logoExts      = map[string]bool{".svg": true, ".png": true, ".jpg": true, ".jpeg": true, ".webp": true}
)

// ParseConfig decodes and validates status.yaml. Unknown fields are errors.
func ParseConfig(data []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, &ValidationError{Problems: []string{ConfigFile + ": " + err.Error()}}
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	var p problems
	s := c.Site
	if strings.TrimSpace(s.Name) == "" {
		p.addf("status.yaml: site.name is required")
	}
	if !isAbsHTTP(s.URL) {
		p.addf("status.yaml: site.url %q must be an absolute http(s) URL", s.URL)
	}
	if s.Timezone == "" {
		p.addf("status.yaml: site.timezone is required, e.g. Europe/Copenhagen")
	} else if loc, err := time.LoadLocation(s.Timezone); err != nil {
		p.addf("status.yaml: site.timezone %q is not a known IANA timezone", s.Timezone)
	} else {
		c.loc = loc
	}
	if s.Logo != "" && (!validAssetPath(s.Logo) || !logoExts[strings.ToLower(path.Ext(s.Logo))]) {
		p.addf("status.yaml: site.logo must be a .svg, .png, .jpg or .webp file inside the repository")
	}
	if s.PrimaryColor != "" && !colorRe.MatchString(s.PrimaryColor) {
		p.addf(`status.yaml: site.primary_color must look like "#0b5fff"`)
	}
	if s.CustomCSS != "" && (!validAssetPath(s.CustomCSS) || path.Ext(s.CustomCSS) != ".css") {
		p.addf("status.yaml: site.custom_css must be a .css file inside the repository")
	}
	for i, l := range s.FooterLinks {
		if strings.TrimSpace(l.Label) == "" {
			p.addf("status.yaml: site.footer_links[%d].label is required", i)
		}
		if !SafeURL(l.URL) {
			p.addf("status.yaml: site.footer_links[%d].url must be an http, https or mailto URL", i)
		}
	}
	if len(c.Components) == 0 {
		p.addf("status.yaml: at least one component is required")
	}
	seen := map[string]bool{}
	for i, comp := range c.Components {
		switch {
		case !componentIDRe.MatchString(comp.ID):
			p.addf("status.yaml: components[%d].id %q must match %s", i, comp.ID, componentIDRe)
		case seen[comp.ID]:
			p.addf("status.yaml: components[%d].id %q is duplicated", i, comp.ID)
		}
		seen[comp.ID] = true
		if strings.TrimSpace(comp.Name) == "" {
			p.addf("status.yaml: components[%d].name is required", i)
		}
	}
	return p.err()
}

func validAssetPath(s string) bool {
	return assetPathRe.MatchString(s) && !strings.Contains(s, "..") && path.Clean(s) == s
}

func isAbsHTTP(u string) bool {
	parsed, err := url.Parse(u)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// SafeURL reports whether u is an absolute http, https or mailto URL.
func SafeURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	case "mailto":
		return parsed.Opaque != ""
	}
	return false
}

// Location is the site's display timezone (UTC if the config was never validated).
func (c *Config) Location() *time.Location {
	if c.loc == nil {
		return time.UTC
	}
	return c.loc
}

func (c *Config) Component(id string) (Component, bool) {
	for _, comp := range c.Components {
		if comp.ID == id {
			return comp, true
		}
	}
	return Component{}, false
}

// ActiveComponents returns the non-retired components in display order.
func (c *Config) ActiveComponents() []Component {
	var out []Component
	for _, comp := range c.Components {
		if !comp.Retired {
			out = append(out, comp)
		}
	}
	return out
}
