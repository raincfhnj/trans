//go:build windows

package main

import (
	"strings"
	"testing"
)

// The command line of the popup process is a contract between two processes
// that never meet: the command that opens a window writes it, and the window
// reads it back. Every switch it accepts — and the ones it refuses — is spelled
// out here, because a switch that stops being written or stops being read is a
// window that opens as something else without anything failing.
func TestThePopupTakesWhatItDrawsFromItsCommandLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    popupOptions
		wantErr string
	}{
		{
			name: "nothing named is the panel",
			args: nil,
			want: popupOptions{},
		},
		{
			name: "the selection window",
			args: []string{"--selection"},
			want: popupOptions{selection: true},
		},
		{
			name: "the settings window",
			args: []string{"--settings"},
			want: popupOptions{settings: true},
		},
		{
			name: "read mode, from the selection in the pane",
			args: []string{"--read", "--capture"},
			want: popupOptions{read: true, capture: true},
		},
		{
			name: "the send key types instead of sending",
			args: []string{"--review"},
			want: popupOptions{sending: sendingReview},
		},
		{
			name: "the send key sends",
			args: []string{"--send"},
			want: popupOptions{sending: sendingSend},
		},
		{
			name: "a panel that closes itself again",
			args: []string{"--probe", "300"},
			want: popupOptions{probe: 300},
		},
		{
			name:    "two windows at once cannot be drawn",
			args:    []string{"--selection", "--settings"},
			wantErr: "different windows",
		},
		{
			name:    "capture belongs to read mode",
			args:    []string{"--capture"},
			wantErr: "--read",
		},
		{
			name:    "the two smaller windows do not translate a draft",
			args:    []string{"--settings", "--read"},
			wantErr: "the panel's own",
		},
		{
			name:    "an unknown switch is refused",
			args:    []string{"--teleport"},
			wantErr: "unknown argument",
		},
		{
			name:    "a probe without its number",
			args:    []string{"--probe"},
			wantErr: "number of milliseconds",
		},
		{
			name:    "a probe that is not a number",
			args:    []string{"--probe", "soon"},
			wantErr: "number of milliseconds",
		},
		{
			name:    "a probe of nothing is not a probe",
			args:    []string{"--probe", "0"},
			wantErr: "number of milliseconds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			options, err := parsePopup(test.args)
			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("%v was accepted, want it refused", test.args)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("%v failed with %v, want it to mention %q",
						test.args, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v was refused: %v", test.args, err)
			}
			if options != test.want {
				t.Errorf("%v reads as %+v, want %+v", test.args, options, test.want)
			}
		})
	}
}

// What the opener writes is what the window reads: the two halves of the
// contract are checked against each other rather than each on its own, which is
// what catches a switch renamed on one side only.
func TestWhatTheOpenerWritesIsWhatTheWindowReads(t *testing.T) {
	t.Parallel()

	wanted := []popupOptions{
		{},
		{selection: true},
		{settings: true},
		{read: true},
		{read: true, capture: true},
		{sending: sendingReview},
		{sending: sendingSend},
		{read: true, sending: sendingReview, probe: 1200},
	}

	for _, options := range wanted {
		written := options.arguments()
		if len(written) == 0 || written[0] != "popup" {
			t.Fatalf("%+v is written as %v, want the popup command first", options, written)
		}

		read, err := parsePopup(written[1:])
		if err != nil {
			t.Fatalf("%+v is written as %v and refused when read back: %v",
				options, written, err)
		}
		if read != options {
			t.Errorf("%+v is written as %v and reads back as %+v", options, written, read)
		}
	}
}

// A panel opened with no command line at all follows the setting; one opened
// with a command line does what it was told, whichever way the setting points.
func TestTheSendKeyFollowsTheCommandLineBeforeTheSetting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options popupOptions
		setting bool
		want    bool
	}{
		{name: "nothing said, the setting sends", options: popupOptions{}, setting: false, want: false},
		{name: "nothing said, the setting types", options: popupOptions{}, setting: true, want: true},
		{name: "told to review over a setting that sends", options: popupOptions{sending: sendingReview}, setting: false, want: true},
		{name: "told to send over a setting that types", options: popupOptions{sending: sendingSend}, setting: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.options.review(test.setting); got != test.want {
				t.Errorf("review(%v) is %v, want %v", test.setting, got, test.want)
			}
		})
	}
}
