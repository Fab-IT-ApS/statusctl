package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"statusctl/model"
	"statusctl/ops"
)

func localRepo(t *testing.T) Local {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "status.yaml"), []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	return Local{Dir: dir}
}

func TestLocalApply(t *testing.T) {
	l := localRepo(t)
	res, err := l.Apply(context.Background(), newIncidentOp("API slow", nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(l.Dir, "incidents", res.Incident.ID+".json")); err != nil {
		t.Fatal(err)
	}
	s, err := l.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Incidents) != 1 || s.Incidents[0].Title != "API slow" {
		t.Errorf("reloaded state = %+v", s.Incidents)
	}
	matches, _ := filepath.Glob(filepath.Join(l.Dir, "incidents", ".statusctl-*"))
	if len(matches) != 0 {
		t.Errorf("temp files left behind: %v", matches)
	}
}

func TestLocalApplyOpErrorWritesNothing(t *testing.T) {
	l := localRepo(t)
	want := &ops.InputError{Msg: "nope"}
	_, err := l.Apply(context.Background(), func(*model.State) (*ops.Result, error) { return nil, want })
	if !errors.Is(err, want) {
		t.Fatalf("op error must come back unchanged, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(l.Dir, "incidents")); !os.IsNotExist(err) {
		t.Error("nothing may be written when the op fails")
	}
}

func TestLocalLoadInvalid(t *testing.T) {
	l := Local{Dir: t.TempDir()}
	_, err := l.Load(context.Background())
	var ve *model.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
}
