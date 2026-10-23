package store

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"statusctl/model"
	"statusctl/ops"
)

// Local reads and writes files in a directory. It does no git operations.
type Local struct {
	Dir string
}

func (l Local) Load(context.Context) (*model.State, error) {
	return model.Load(os.DirFS(l.Dir))
}

func (l Local) Apply(ctx context.Context, op Op) (*ops.Result, error) {
	s, err := l.Load(ctx)
	if err != nil {
		return nil, err
	}
	res, err := op(s)
	if err != nil {
		return nil, err
	}
	for _, p := range slices.Sorted(maps.Keys(res.Files)) {
		if err := writeFileAtomic(filepath.Join(l.Dir, filepath.FromSlash(p)), res.Files[p]); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func writeFileAtomic(name string, data []byte) error {
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".statusctl-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}
