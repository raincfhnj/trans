//go:build !windows

package wintarget

import (
	"context"
	"errors"
	"testing"
)

// Off Windows there is no pane to paste into, and the two targets say so rather
// than reporting a delivery that never happened: a prompt that silently went
// nowhere is the worst of the answers available.
func TestADeliveryOffWindowsSaysSo(t *testing.T) {
	t.Parallel()

	sending := NewSending(0x1234, "ctrl+v")
	typing := NewTyping(0x1234, "ctrl+v")

	for name, err := range map[string]error{
		"sending": sending.Insert(context.Background(), "a prompt"),
		"typing":  typing.Insert(context.Background(), "a prompt"),
	} {
		if err == nil {
			t.Errorf("%s reported a delivery it cannot make", name)
			continue
		}
		if !errors.Is(err, errWindowsOnly) {
			t.Errorf("%s answered %v, want the refusal itself", name, err)
		}
	}
}
