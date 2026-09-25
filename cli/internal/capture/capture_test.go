package capture

import (
	"context"
	"testing"
	"time"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
)

func TestCaptureSession_CreateAndCancel(t *testing.T) {
	opts := Options{
		Mode: ModeCG,
	}
	session := NewSession(opts)
	if session == nil {
		t.Fatalf("expected non-nil session")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := session.Start(ctx, func(ev analyzer.Event) {})
	if err != nil {
		t.Fatalf("session.Start returned unexpected error: %v", err)
	}
}
