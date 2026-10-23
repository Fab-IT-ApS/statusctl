package ops

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Fab-IT-ApS/statusctl/model"
)

const testConfig = `site:
  name: Test Status
  url: "https://status.example.com"
  timezone: Europe/Copenhagen
components:
  - { id: api, name: Public API, group: Platform }
  - { id: web, name: Web portal, group: Platform }
  - { id: mail, name: Email }
  - { id: ftp, name: FTP, retired: true }
`

var now = time.Date(2026, 10, 23, 9, 12, 30, 0, time.UTC)

func testState(t *testing.T, incidents ...*model.Incident) *model.State {
	t.Helper()
	cfg, err := model.ParseConfig([]byte(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	return &model.State{Config: cfg, Incidents: incidents}
}

func wantInputError(t *testing.T, err error, want string) {
	t.Helper()
	var ie *InputError
	if !errors.As(err, &ie) {
		t.Fatalf("want *InputError containing %q, got %v", want, err)
	}
	if !strings.Contains(ie.Msg, want) {
		t.Errorf("error %q does not mention %q", ie.Msg, want)
	}
}

func openIncident(t *testing.T, s *model.State) *model.Incident {
	t.Helper()
	res, err := NewIncident(s, NewIncidentArgs{
		Title:   "Elevated API latency",
		Impacts: map[string]model.Impact{"web": model.PartialOutage, "api": model.Degraded},
		Message: "Investigating.", Author: "Ada",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return res.Incident
}
