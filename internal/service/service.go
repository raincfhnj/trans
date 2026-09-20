// Package service decides what a draft goes through. The question is the same
// wherever the popup is opened from — a herdr pane or a window on this machine —
// so it is answered in one place rather than in every entry point.
package service

import (
	"errors"
	"fmt"

	"trans/internal/command"
	"trans/internal/config"
	"trans/internal/deepl"
	"trans/internal/google"
	"trans/internal/gtranslate"
	"trans/internal/mymemory"
	"trans/internal/openai"
	"trans/internal/translation"
)

// Choice is the service a popup was given, and what to say about it when it
// could not be built.
type Choice struct {
	Name       string
	Translator translation.Translator
	// Translates says the popup has something to translate with, so it offers the
	// keys and the second panel that go with it.
	Translates bool
	// Trouble is a service that was asked for and could not be built, which the
	// popup says out loud.
	Trouble error
}

// Registry lists the translation services this build offers. The first one is
// the default; adding another service means adding it here.
func Registry() *translation.Registry {
	return translation.NewRegistry(
		deepl.Provider{},
		google.Provider{},
		gtranslate.Provider{},
		openai.Provider{},
		mymemory.Provider{},
		command.Provider{},
		translation.DryRunProvider{},
		translation.OffProvider{},
	)
}

// Choose decides what the draft goes through. A key on its own means the default
// service, a command on its own the program it names; no key, no command and no
// choice means none at all, and the popup is then the draft box it always was.
func Choose(settings *config.Settings) Choice {
	return ChooseWith(Registry(), settings)
}

// ChooseWith is Choose against a registry of someone else's making, which is how
// a test drives one service without the others.
func ChooseWith(registry *translation.Registry, settings *config.Settings) Choice {
	name, err := chosenName(settings, registry)
	if err == nil {
		var translator translation.Translator
		translator, err = registry.Translator(name, settings.Options)
		if err == nil {
			return Choice{
				Name:       name,
				Translator: translator,
				Translates: name != translation.OffProvider{}.Name(),
			}
		}
	}

	if settings.ConfigFile != "" {
		err = fmt.Errorf("%w; configure it in %s", err, settings.ConfigFile)
	}
	return Choice{
		Name:       name,
		Translator: translation.Broken{Why: err},
		Translates: true,
		Trouble:    err,
	}
}

// A key and a command are two answers to the same question, and picking one of
// them would send the draft somewhere it was not meant to go.
func chosenName(settings *config.Settings, registry *translation.Registry) (string, error) {
	if settings.Provider != "" {
		return settings.Provider, nil
	}

	hasKey, hasCommand := settings.Options.APIKey != "", settings.Options.Command != ""
	switch {
	case hasKey && hasCommand:
		return "", errors.New(
			"there is both an API key and a command, so it is unclear which translates: " +
				"choose one with TRANS_PROVIDER")
	case hasCommand:
		return command.Provider{}.Name(), nil
	case hasKey:
		return registry.Default(), nil
	default:
		return translation.OffProvider{}.Name(), nil
	}
}
