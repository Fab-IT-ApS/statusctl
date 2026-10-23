package model

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
)

// State is everything in one organisation's data repository.
type State struct {
	Config    *Config
	Incidents []*Incident // sorted by ID
}

func (s *State) Incident(id string) *Incident {
	for _, in := range s.Incidents {
		if in.ID == id {
			return in
		}
	}
	return nil
}

// Load reads and fully validates a data repository. Any problem fails the
// whole load: a corrupt file must be fixed, never silently skipped.
func Load(fsys fs.FS) (*State, error) {
	data, err := fs.ReadFile(fsys, ConfigFile)
	if err != nil {
		return nil, &ValidationError{Problems: []string{fmt.Sprintf("%s: %v", ConfigFile, err)}}
	}
	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, err
	}
	var p problems
	for _, f := range []struct{ field, path string }{
		{"site.logo", cfg.Site.Logo}, {"site.custom_css", cfg.Site.CustomCSS},
	} {
		if f.path == "" {
			continue
		}
		if _, err := fs.Stat(fsys, f.path); err != nil {
			p.addf("status.yaml: %s %q does not exist in the repository", f.field, f.path)
		}
	}
	s := &State{Config: cfg}
	entries, err := fs.ReadDir(fsys, IncidentDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".json" {
			continue
		}
		name := IncidentDir + "/" + e.Name()
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		in, err := ParseIncident(b)
		if err != nil {
			p.addf("%s: %v", name, err)
			continue
		}
		cfg.validateIncident(in, name, &p)
		s.Incidents = append(s.Incidents, in)
	}
	if err := p.err(); err != nil {
		return nil, err
	}
	return s, nil
}
