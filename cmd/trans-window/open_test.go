//go:build windows

package main

import (
	"strings"
	"testing"
)

// The command line is the whole interface to opening the panel, so every word
// it accepts — and the ones it refuses — is spelled out here.
func TestOpenTakesReadCaptureTargetAndProbeFromTheCommandLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    openOptions
		wantErr string
	}{
		{
			name: "open alone is the panel over the window in front",
			args: nil,
			want: openOptions{submit: true},
		},
		{
			name: "review only types the prompt in",
			args: []string{"--review"},
			want: openOptions{},
		},
		{
			name: "read turns the panel around",
			args: []string{"--read"},
			want: openOptions{submit: true, read: true},
		},
		{
			name: "capture starts the draft from the selection",
			args: []string{"--read", "--capture"},
			want: openOptions{submit: true, read: true, capture: true},
		},
		{
			name: "a window may be named",
			args: []string{"--target", "0x1234"},
			want: openOptions{submit: true, target: "0x1234"},
		},
		{
			name: "the probe is passed on",
			args: []string{"--probe", "300"},
			want: openOptions{submit: true, probe: "300"},
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseOpen(test.args)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("parseOpen(%v) returned %v, want an error mentioning %q",
						test.args, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOpen(%v) returned unexpected error: %v", test.args, err)
			}
			if got != test.want {
				t.Errorf("parseOpen(%v) = %+v, want %+v", test.args, got, test.want)
			}
		})
	}
}
