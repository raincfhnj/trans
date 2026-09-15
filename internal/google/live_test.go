//go:build live

package google_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"trans/internal/config"
	"trans/internal/google"
	"trans/internal/translation"
)

// A check against the real service, kept behind a build tag so the ordinary suite
// needs no key and no network:
//
//	HERDR_PLUGIN_CONFIG_DIR="$(herdr plugin config-dir local.trans)" \
//	HERDR_TRANS_TARGET=w1:p1 HERDR_TRANS_PROVIDER=google \
//	go test -tags live -run Live -v ./internal/google/
func TestLiveTranslationThroughTheRealService(t *testing.T) {
	settings, err := config.Load(os.Getenv)
	if err != nil {
		t.Fatalf("reading the settings: %v", err)
	}
	if settings.Options.APIKey == "" {
		t.Skip("no API key configured for google")
	}

	translator, err := google.Provider{}.New(settings.Options)
	if err != nil {
		t.Fatalf("building the translator: %v", err)
	}

	const draft = "Bitte behebe den fehlschlagenden Test im Warenkorb. Nutze Tabellentests."
	english, err := translator.Translate(context.Background(), draft)
	if err != nil {
		t.Fatalf("Translate returned: %v", err)
	}
	if strings.TrimSpace(english) == "" || english == draft {
		t.Errorf("Translate returned %q, want English", english)
	}
	t.Logf("German:  %s", draft)
	t.Logf("English: %s", english)

	if _, err := translation.ReporterOf(translator); err == nil {
		t.Log("the service reports usage")
	} else {
		t.Logf("usage: %v", err)
	}
}
