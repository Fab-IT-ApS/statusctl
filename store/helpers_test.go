package store

import (
	"time"

	"statusctl/model"
	"statusctl/ops"
)

const testConfig = `site:
  name: Test Status
  url: "https://status.example.com"
  timezone: Europe/Copenhagen
components:
  - { id: api, name: Public API }
  - { id: web, name: Web portal }
`

var testNow = time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)

func newIncidentOp(title string, calls *int) Op {
	return func(s *model.State) (*ops.Result, error) {
		if calls != nil {
			*calls++
		}
		return ops.NewIncident(s, ops.NewIncidentArgs{
			Title: title, Impacts: map[string]model.Impact{"api": model.Degraded},
			Message: "Investigating.", Author: "Ada",
		}, testNow)
	}
}
