//go:build !darwin

package capture

import (
	"context"
	"fmt"
)

type otherSession struct {
	opts Options
}

// NewSession returns a non-Darwin stub session.
func NewSession(opts Options) Session {
	return &otherSession{opts: opts}
}

// Start returns an error on non-Darwin platforms.
func (s *otherSession) Start(ctx context.Context, handler Handler) error {
	return fmt.Errorf("live capture is currently supported on macOS")
}
