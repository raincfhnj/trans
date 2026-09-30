package service

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"trans/internal/config"
	"trans/internal/translation"
)

func TestWithoutAKeyThePopupStillOpensAsAPlainDraftBox(t *testing.T) {
	chosen := Choose(&config.Settings{})

	if chosen.Name != "off" {
		t.Errorf("without a key the service is %q, want it off", chosen.Name)
	}
	if chosen.Trouble != nil {
		t.Errorf("without a key there is trouble to report: %v", chosen.Trouble)
	}
	if chosen.Translates {
		t.Error("without a key the popup claims it translates")
	}

	written := "Bitte behebe den fehlschlagenden Test"
	delivered, err := chosen.Translator.Translate(context.Background(), written)
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if delivered != written {
		t.Errorf("Translate returned %q, want the draft as it was written", delivered)
	}
}

func TestAKeyOnItsOwnPicksTheDefaultService(t *testing.T) {
	chosen := Choose(&config.Settings{
		Options: translation.Options{APIKey: "key-123", TargetLanguage: "EN-US"},
	})

	if chosen.Name != "deepl" {
		t.Errorf("with a key the service is %q, want the default", chosen.Name)
	}
	if !chosen.Translates {
		t.Error("with a key the popup does not claim to translate")
	}
	if chosen.Trouble != nil {
		t.Errorf("with a key there is trouble to report: %v", chosen.Trouble)
	}
}

// Asking for a service that cannot be built is not the same as asking for none:
// the draft must never reach an agent untranslated because a key was missing.
func TestAServiceThatCannotBeBuiltSaysSoAndRefusesToTranslate(t *testing.T) {
	chosen := Choose(&config.Settings{
		Provider:   "deepl",
		ConfigFile: "/somewhere/.env",
	})

	if chosen.Trouble == nil {
		t.Fatal("a service that cannot be built reports no trouble")
	}
	if !strings.Contains(chosen.Trouble.Error(), "/somewhere/.env") {
		t.Errorf("the trouble is %v, want it to say where the key belongs", chosen.Trouble)
	}
	if !chosen.Translates {
		t.Error("the popup pretends translation is off when the service is only broken")
	}

	if _, err := chosen.Translator.Translate(context.Background(), "Bitte behebe es"); err == nil {
		t.Error("the broken service translated something instead of saying why it cannot")
	}
}

// A command needs no key, so setting one is enough to choose it: there is nothing
// else it could mean.
func TestACommandOnItsOwnPicksTheLocalService(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command in this test is a unix program")
	}

	chosen := Choose(&config.Settings{
		Options: translation.Options{Command: "sed 's/behebe/fix/'", TargetLanguage: "EN-US"},
	})

	if chosen.Name != "cmd" {
		t.Errorf("with a command the service is %q, want cmd", chosen.Name)
	}
	if !chosen.Translates {
		t.Error("with a command the popup does not claim to translate")
	}
	if chosen.Trouble != nil {
		t.Errorf("with a command there is trouble to report: %v", chosen.Trouble)
	}

	translated, err := chosen.Translator.Translate(context.Background(), "Bitte behebe es")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if translated != "Bitte fix es" {
		t.Errorf("Translate returned %q, want what the command answered", translated)
	}
}

// A key and a command together say nothing about which was meant, and guessing
// wrong would send a draft to a service the person thought they had left behind.
func TestAKeyAndACommandTogetherAreRefusedRatherThanGuessedAt(t *testing.T) {
	chosen := Choose(&config.Settings{
		Options: translation.Options{APIKey: "key-123", Command: "cat"},
	})

	if chosen.Trouble == nil {
		t.Fatal("a key and a command together report no trouble")
	}
	if !strings.Contains(chosen.Trouble.Error(), "TRANS_PROVIDER") {
		t.Errorf("the trouble is %v, want it to say which setting decides", chosen.Trouble)
	}
	if _, err := chosen.Translator.Translate(context.Background(), "Bitte behebe es"); err == nil {
		t.Error("something was translated while it is unclear by what")
	}
}

func TestAnExplicitServiceIsUsedAsAsked(t *testing.T) {
	chosen := Choose(&config.Settings{Provider: "dry-run"})

	if chosen.Name != "dry-run" {
		t.Errorf("the service is %q, want the one asked for", chosen.Name)
	}
	translated, err := chosen.Translator.Translate(context.Background(), "Bitte behebe es")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if !strings.Contains(translated, "dry-run") {
		t.Errorf("Translate returned %q, want the dry-run marker", translated)
	}
}

// The free service is the one to reach for when nothing has been set up, so it
// has to be in the registry under the name the settings use.
func TestAServiceThatNeedsNoKeyIsAnsweredWithoutAKey(t *testing.T) {
	chosen := Choose(&config.Settings{Provider: "gtranslate"})

	if !chosen.Translates {
		t.Error("the service without a key does not claim to translate")
	}
	if chosen.Trouble != nil {
		t.Errorf("the service without a key reports trouble: %v", chosen.Trouble)
	}
}

// The README documents TRANS_PROVIDER with the default `auto`, so a user who
// follows it must land on the automatic choice. No registry holds a service by
// that name, and asking for one breaks every translation.
func TestAutoProviderNameMeansTheAutomaticChoiceRatherThanAService(t *testing.T) {
	registry := Registry()

	for _, test := range []struct {
		name        string
		options     translation.Options
		wantName    string
		wantTranses bool
	}{
		{
			name:        "a key picks the default service",
			options:     translation.Options{APIKey: "key-123", TargetLanguage: "EN-US"},
			wantName:    "deepl",
			wantTranses: true,
		},
		{
			name:        "a command picks the local service",
			options:     translation.Options{Command: "translate-the-draft"},
			wantName:    "cmd",
			wantTranses: true,
		},
		{
			name:        "neither picks off",
			options:     translation.Options{},
			wantName:    "off",
			wantTranses: false,
		},
	} {
		automatic := ChooseWith(registry, &config.Settings{Options: test.options})
		if automatic.Name != test.wantName || automatic.Translates != test.wantTranses {
			t.Errorf("%s: the automatic choice is %q (translates=%t), want %q (translates=%t)",
				test.name, automatic.Name, automatic.Translates, test.wantName, test.wantTranses)
		}
		if automatic.Trouble != nil {
			t.Errorf("%s: the automatic choice reports trouble: %v", test.name, automatic.Trouble)
		}

		for _, provider := range []string{"auto", "Auto", " AUTO "} {
			chosen := ChooseWith(registry, &config.Settings{Provider: provider, Options: test.options})

			if chosen.Trouble != nil {
				t.Errorf("%s with Provider %q reports trouble: %v", test.name, provider, chosen.Trouble)
			}
			if chosen.Name != automatic.Name || chosen.Translates != automatic.Translates {
				t.Errorf("%s with Provider %q chose %q (translates=%t), want the same as no provider: %q (translates=%t)",
					test.name, provider, chosen.Name, chosen.Translates,
					automatic.Name, automatic.Translates)
			}
		}
	}
}
