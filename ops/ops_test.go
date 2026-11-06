package ops

import (
	"testing"

	"github.com/Fab-IT-ApS/statusctl/model"
)

func TestCommitMessageAction(t *testing.T) {
	s := testState(t)
	res, err := NewIncident(s, NewIncidentArgs{
		Title: "X", Impacts: map[string]model.Impact{"api": model.Degraded}, Message: "m", Author: "Ada",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.CommitMessage(); got != "incident 2026-10-23-x: investigating" {
		t.Errorf("CommitMessage = %q", got)
	}
	if got := res.CommitAuthorName(); got != "Ada" {
		t.Errorf("CommitAuthorName = %q, want the latest update's author", got)
	}
	res.Action, res.CommitAuthor = "retracted by Bo", "Bo"
	if got := res.CommitMessage(); got != "incident 2026-10-23-x: retracted by Bo" {
		t.Errorf("CommitMessage = %q", got)
	}
	if got := res.CommitAuthorName(); got != "Bo" {
		t.Errorf("CommitAuthorName = %q", got)
	}
}
