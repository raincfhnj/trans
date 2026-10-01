//go:build windows

// The command translator on the platform it ships on: a command line run
// through cmd.exe. The POSIX half of its tests needs sed and a shell that is
// not there on Windows, so the behaviour that matters is checked twice — here
// with the tools every Windows install has, and there with the ones a Unix
// user has.
package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"trans/internal/command"
)

// The draft goes in on stdin and the command's answer comes back without the
// newline cmd.exe leaves on it, which is what a translation is.
func TestTheDraftIsWrittenToTheWindowsShellAndItsAnswerRead(t *testing.T) {
	translator := command.New("findstr .")

	translated, err := translator.Translate(context.Background(), "Bitte behebe den Test")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if translated != "Bitte behebe den Test" {
		t.Errorf("Translate returned %q, want the draft the command echoed back", translated)
	}
}

// The language is handed to the command in its environment, because a script
// that is not this program's may still want to read it. The command line is the
// one place a shell expands it, so that is where the test looks.
func TestTheCommandIsToldTheTargetLanguage(t *testing.T) {
	translator := command.New("echo %TRANS_TARGET_LANGUAGE%", command.WithTargetLanguage("EN-GB"))

	translated, err := translator.Translate(context.Background(), "Bitte behebe den Test")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if translated != "EN-GB" {
		t.Errorf("the command saw %q, want the language it was told", translated)
	}
}

// A command that fails says so with what it complained about: the complaint is
// the only clue to why, and it is what the popup shows.
func TestAWindowsCommandThatFailsIsReportedWithItsComplaint(t *testing.T) {
	translator := command.New(`echo model de-en not installed 1>&2 & exit 3`)

	_, err := translator.Translate(context.Background(), "Bitte behebe den Test")

	if err == nil {
		t.Fatal("Translate returned no error for a command that failed")
	}
	if !strings.Contains(err.Error(), "model de-en not installed") {
		t.Errorf("Translate says %v, want what the command complained about", err)
	}
}

// A command that hangs is not allowed to hold the popup: the deadline ends it,
// and what comes back says that rather than blaming the command.
func TestACommandThatHangsIsGivenUpOn(t *testing.T) {
	translator := command.New("ping -n 30 127.0.0.1 >nul", command.WithTimeout(150*time.Millisecond))

	started := time.Now()
	_, err := translator.Translate(context.Background(), "Bitte behebe den Test")

	if err == nil {
		t.Fatal("Translate returned no error for a command that never answered")
	}
	if !strings.Contains(err.Error(), "did not answer in time") {
		t.Errorf("Translate says %v, want the deadline named", err)
	}
	if waited := time.Since(started); waited > 10*time.Second {
		t.Errorf("Translate waited %s, want it to give up on its own deadline", waited)
	}
}

// A command that returns nothing is a failure rather than an empty translation:
// an empty prompt is not something to hand to an agent.
func TestACommandThatReturnsNothingIsAFailure(t *testing.T) {
	translator := command.New("exit 0")

	_, err := translator.Translate(context.Background(), "Bitte behebe den Test")

	if err == nil {
		t.Fatal("Translate returned no error for a command with nothing to say")
	}
	if !strings.Contains(err.Error(), "returned nothing") {
		t.Errorf("Translate says %v, want it to say there was no answer", err)
	}
}

// A program that is not installed is reported as one that could not be run,
// which is a different thing to fix from one that refused the draft.
func TestACommandThatCannotBeFoundIsReportedAsSuch(t *testing.T) {
	translator := command.New("trans-no-such-program-here")

	_, err := translator.Translate(context.Background(), "Bitte behebe den Test")

	if err == nil {
		t.Fatal("Translate returned no error for a command that is not installed")
	}
	if !strings.Contains(err.Error(), "trans-no-such-program-here") {
		t.Errorf("Translate says %v, want the command named", err)
	}
}

// A cancelled translation is the panel being closed while the command was still
// running: the answer is nobody's, and the reason is the cancellation rather
// than a command that failed.
func TestACancelledTranslationSaysItWasCancelled(t *testing.T) {
	translator := command.New("ping -n 30 127.0.0.1 >nul")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := translator.Translate(ctx, "Bitte behebe den Test")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Translate returned %v, want the cancellation", err)
	}
}
