//go:build windows

package win32

import (
	"context"
	"testing"
	"time"
)

// waitForClipboardChange gives up when the clipboard is not written, rather than
// waiting for something that is not coming.
func TestWaitingForTheClipboardGivesUpWhenNothingWrites(t *testing.T) {
	started := time.Now()
	if waitForClipboardChange(context.Background()) {
		t.Skip("something else wrote the clipboard while this test ran")
	}
	if waited := time.Since(started); waited > 5*time.Second {
		t.Errorf("waitForClipboardChange waited %s, want a bounded wait", waited)
	}
}

// A cancelled context is answered at once rather than after the bound, which is
// what the panel asks for when the author presses escape.
func TestWaitingForTheClipboardStopsOnACancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	waitForClipboardChange(ctx)
	if waited := time.Since(started); waited > time.Second {
		t.Errorf("waitForClipboardChange waited %s after cancellation, want an immediate answer", waited)
	}
}
