//go:build windows

package wintarget_test

import (
	"context"
	"testing"

	"trans/internal/wintarget"
)

// The two targets the panel is handed work through the delivery they were built
// for. A prompt with no window to go to is refused before the clipboard is
// borrowed, so this asks for that error and no more: it is the check that the
// exported constructors carry the handle on to the delivery, and it touches no
// clipboard, no window and no key of this machine.
func TestTheTargetsBuiltForThePanelRefuseAWindowThatIsNotThere(t *testing.T) {
	t.Parallel()

	targets := []struct {
		name   string
		target interface {
			Insert(ctx context.Context, text string) error
		}
	}{
		{name: "sending", target: wintarget.NewSending(0, "ctrl+v")},
		{name: "typing", target: wintarget.NewTyping(0, "ctrl+v")},
	}

	for _, test := range targets {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.target.Insert(context.Background(), "a prompt")
			if err == nil {
				t.Fatal("Insert answered no error for a window that is not there")
			}
			if got := err.Error(); got != "no window to deliver the prompt to" {
				t.Errorf("Insert answered %q, want the error for a window that is not there", got)
			}
		})
	}
}
