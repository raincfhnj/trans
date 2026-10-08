//go:build windows

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// The command line of a panel in a terminal is the whole interface to it, so
// every word it accepts �� and the ones it refuses �� is spelled out here.
func TestTUITakesWhatItDrawsFromItsCommandLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    tuiOptions
		wantErr string
	}{
		{
			name: "tui alone follows the send setting",
			args: nil,
			want: tuiOptions{},
		},
		{
			name: "review only types the prompt in",
			args: []string{"--review"},
			want: tuiOptions{sending: sendingReview},
		},
		{
			name: "send says what the send key does",
			args: []string{"--send"},
			want: tuiOptions{sending: sendingSend},
		},
		{
			name: "read turns the panel around",
			args: []string{"--read"},
			want: tuiOptions{read: true},
		},
		{
			name: "capture starts the draft from the selection",
			args: []string{"--read", "--capture"},
			want: tuiOptions{read: true, capture: true},
		},
		{
			name: "inline keeps the terminal's own screen",
			args: []string{"--inline"},
			want: tuiOptions{inline: true},
		},
		{
			name: "beside delivers into the pane it was split from",
			args: []string{"--beside"},
			want: tuiOptions{beside: true},
		},
		{
			name: "a window to deliver into may be named",
			args: []string{"--target", "0x1234"},
			want: tuiOptions{target: "0x1234"},
		},
		{
			name: "the probe is passed on",
			args: []string{"--probe", "300"},
			want: tuiOptions{probe: 300},
		},
		{
			name:    "capture belongs to read mode",
			args:    []string{"--capture"},
			wantErr: "--read",
		},
		{
			name:    "an unknown argument is refused",
			args:    []string{"--teleport"},
			wantErr: "unknown argument",
		},
		{
			name:    "--target needs the window it names",
			args:    []string{"--target"},
			wantErr: "--target needs",
		},
		{
			name:    "--probe needs the number of milliseconds",
			args:    []string{"--probe"},
			wantErr: "--probe needs",
		},
		{
			name:    "--probe wants a number",
			args:    []string{"--probe", "soon"},
			wantErr: "--probe wants",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseTUI(test.args)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("parseTUI(%v) returned %v, want an error mentioning %q",
						test.args, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTUI(%v) returned unexpected error: %v", test.args, err)
			}
			if got != test.want {
				t.Errorf("parseTUI(%v) = %+v, want %+v", test.args, got, test.want)
			}
		})
	}
}

// The send key does what the command line said, and follows the setting only
// when the command line said nothing �� a panel in a terminal answers to the
// same words the popup does.
func TestTheSendKeyOfATUIFollowsTheSettingOnlyWhenToldNothing(t *testing.T) {
	t.Parallel()

	plain := tuiOptions{}
	if !plain.review(true) || plain.review(false) {
		t.Error("a tui with no switch on its command line does not follow TRANS_SUBMIT")
	}
	reviewing := tuiOptions{sending: sendingReview}
	if !reviewing.review(false) {
		t.Error("--review does not type the prompt in")
	}
	sending := tuiOptions{sending: sendingSend}
	if sending.review(true) {
		t.Error("--send does not send")
	}
}

// A delivery beside the agent crosses to that pane and back: the prompt is
// pasted over there, and the keyboard returns to the panel only when the
// prompt was sent �� a prompt that is only typed in keeps the focus where the
// author has to press the last keystroke.
func TestADeliveryBesideCrossesToTheOtherPane(t *testing.T) {
	t.Parallel()

	type record struct{ what string }
	see := func(t *testing.T, returnFocus bool, insertErr error) []record {
		t.Helper()
		seen := []record{}
		target := besideTarget{
			inner:       recording{append: func(text string) error { seen = append(seen, record{"paste:" + text}); return insertErr }},
			returnFocus: returnFocus,
			move:        func(direction string) error { seen = append(seen, record{"move:" + direction}); return nil },
			wait:        func(time.Duration) { seen = append(seen, record{"wait"}) },
		}
		_ = target.Insert(context.Background(), "prompt")
		return seen
	}

	sent := see(t, true, nil)
	want := []record{{"move:previous"}, {"wait"}, {"paste:prompt"}, {"move:previous"}}
	if len(sent) != len(want) {
		t.Fatalf("a sent delivery did %v, want %v", sent, want)
	}
	for index := range want {
		if sent[index] != want[index] {
			t.Fatalf("a sent delivery did %v, want %v", sent, want)
		}
	}

	typed := see(t, false, nil)
	if len(typed) != 3 || typed[2].what != "paste:prompt" {
		t.Errorf("a typed-in delivery did %v, want the focus left over there", typed)
	}

	// The prompt could not be pasted. The words that say so are drawn in the
	// panel, so the keyboard comes back to it however the delivery went.
	failed := see(t, true, errors.New("lost focus"))
	if len(failed) != 4 || failed[3].what != "move:previous" {
		t.Errorf("a failed delivery did %v, want the focus back at the panel", failed)
	}

	// The crossing itself is the one thing a delivery cannot do without.
	crossed := false
	stuck := besideTarget{
		inner: recording{append: func(string) error { crossed = true; return nil }},
		move:  func(string) error { return errors.New("no pane there") },
		wait:  func(time.Duration) {},
	}
	if err := stuck.Insert(context.Background(), "prompt"); err == nil || crossed {
		t.Errorf("a delivery crossed no pane and still pasted: crossed=%v err=%v", crossed, err)
	}
}

type recording struct {
	append func(text string) error
}

func (r recording) Insert(_ context.Context, text string) error { return r.append(text) }
