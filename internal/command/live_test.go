//go:build live

package command_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"trans/internal/command"
)

// A check against a real translator on this machine, kept behind a build tag so
// the ordinary suite needs nothing installed:
//
//	HERDR_TRANS_COMMAND="/Applications/translateLocally.app/Contents/MacOS/translateLocally -m de-en-base" \
//	go test -tags live -run Live -v ./internal/command/
func TestLiveTranslationThroughARealCommand(t *testing.T) {
	commandLine := os.Getenv("HERDR_TRANS_COMMAND")
	if commandLine == "" {
		t.Skip("no HERDR_TRANS_COMMAND configured")
	}

	translator := command.New(commandLine, command.WithTargetLanguage("EN-US"))

	started := time.Now()
	translated, err := translator.Translate(context.Background(),
		"Bitte behebe den fehlschlagenden Test und erklär mir danach, was falsch war.")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	t.Logf("%s (in %v)", translated, time.Since(started).Round(time.Millisecond))

	for _, word := range []string{"test", "fix"} {
		if !strings.Contains(strings.ToLower(translated), word) {
			t.Errorf("the translation is %q, want English with %q in it", translated, word)
		}
	}
}
