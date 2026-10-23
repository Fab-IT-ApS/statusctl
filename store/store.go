// Package store reads and writes a data repository, either a local directory
// or a GitHub repository through its REST API.
package store

import (
	"context"
	"errors"

	"github.com/Fab-IT-ApS/statusctl/model"
	"github.com/Fab-IT-ApS/statusctl/ops"
)

// Op computes a change from current state. It may run more than once when a
// write conflicts, so it must be a pure function of its input.
type Op func(*model.State) (*ops.Result, error)

type Store interface {
	Load(ctx context.Context) (*model.State, error)
	Apply(ctx context.Context, op Op) (*ops.Result, error)
}

var (
	ErrConflict = errors.New("the repository kept changing underneath us; nothing was written")
	ErrAuth     = errors.New("authentication failed")
	ErrNetwork  = errors.New("network error")
)
