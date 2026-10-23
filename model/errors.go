package model

import (
	"fmt"
	"strings"
)

// ValidationError lists every problem found in a repository's data.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid status data:\n  " + strings.Join(e.Problems, "\n  ")
}

type problems []string

func (p *problems) addf(format string, args ...any) {
	*p = append(*p, fmt.Sprintf(format, args...))
}

func (p problems) err() error {
	if len(p) == 0 {
		return nil
	}
	return &ValidationError{Problems: p}
}
