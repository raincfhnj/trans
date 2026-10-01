//go:build !windows

package main

import (
	"strings"
	"testing"
)

// Off Windows there is no setup to run and no voice to find. The three
// functions exist so that this package has the same shape everywhere; what they
// owe a caller is the truth rather than silence, and that is what this checks.
func TestTheSetupWizardSaysItIsForWindowsOnly(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{nil, {"doctor"}, {"--json"}, {"doctor", "--json"}} {
		err := runSetup(arguments)
		if err == nil {
			t.Errorf("runSetup(%v) returned no error, want the refusal", arguments)
			continue
		}
		if !strings.Contains(err.Error(), "Windows") {
			t.Errorf("runSetup(%v) failed with %v, want it to say why", arguments, err)
		}
	}
}

// A lookup that cannot happen answers nothing rather than a path nobody can
// use: a caller that went on to open it would fail somewhere further away.
func TestThePathLookupAnswersNothingOffWindows(t *testing.T) {
	t.Parallel()

	found, err := execLookPathImpl("trans-window.exe")
	if err == nil {
		t.Error("execLookPathImpl returned no error, want the refusal")
	}
	if found != "" {
		t.Errorf("execLookPathImpl returned %q, want nothing", found)
	}
}

// Speech synthesis is a Windows facility; saying there is none is the honest
// answer, and it is a warning rather than a failure everywhere it is reported.
func TestSpeechIsReportedAsUnavailableOffWindows(t *testing.T) {
	t.Parallel()

	detail, available := speechCheck()
	if available {
		t.Error("speechCheck says a voice is available off Windows")
	}
	if detail != "" {
		t.Errorf("speechCheck said %q, want nothing", detail)
	}
}
