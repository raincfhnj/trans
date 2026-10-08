// Package settings is the window that edits the plugin's own settings: the
// service and its options, how the panel behaves, and the chords the daemon
// waits for. It writes the .env file in the config directory and never the
// environment — a variable set there wins over the file, and the window marks
// those rows rather than pretending otherwise.
package settings

import (
	"strconv"
	"strings"

	"trans/internal/config"
	"trans/internal/frame"
)

// kind is what a row does with the keys: written with enter, stepped through
// with the arrows, or switched like a flag.
type kind int

const (
	text kind = iota
	// secret is written the same way but never shown as it stands.
	secret
	choice
	flag
	// chord is a key combination that must name one; a hotkey may also be off.
	chord
	hotkey
	// number is a whole number between min and max: the arrows step it and
	// enter writes it.
	number
)

type row struct {
	label    string
	variable string
	kind     kind
	// value is the setting as it would be written: flags keep 1 or 0, and the
	// service row keeps nothing for auto, so saving it takes the line out.
	value    string
	original string
	// env says the variable is set in the environment, which wins over the
	// file this window writes.
	env bool
	// choices are the values a choice row steps through; auto comes first and
	// stands for nothing.
	choices []string
	// min and max are the two ends a number row stands between.
	min, max int
	// empty is what a row with no value says instead of showing a gap.
	empty string
}

func (row *row) changed() bool { return row.value != row.original }

// shown is the value as it reads in the list.
func (row *row) shown() string {
	switch {
	case row.kind == choice && row.value == "":
		return "auto"
	case row.value != "":
		return row.value
	case row.empty != "":
		return row.empty
	default:
		return "—"
	}
}

// buildRows is every setting this window offers, in the order they read:
// what the translation goes through, how the panel behaves, how it looks and
// how much room it takes, and the chords that call it up.
func buildRows(settings config.Settings, options Options) []*row {
	// The key is read from the service's own variable when a service is named,
	// and written back to the same place.
	keyVariable := config.ScopedKeyVar(settings.Provider)
	if keyVariable == "" {
		keyVariable = config.ApiKeyVar
	}

	rows := []*row{
		{
			label: "service", variable: config.ProviderVar, kind: choice,
			choices: append([]string{"auto"}, options.Services...),
			value:   settings.Provider,
		},
		{
			label: "api key", variable: keyVariable, kind: secret,
			value: settings.Options.APIKey, empty: "not set",
		},
		{
			label: "endpoint", variable: config.EndpointVar, kind: text,
			value: settings.Options.Endpoint, empty: "the service's own",
		},
		{
			label: "model", variable: config.ModelVar, kind: text,
			value: settings.Options.Model, empty: "the service's own",
		},
		{
			label: "language", variable: config.LanguageVar, kind: text,
			value: settings.Options.TargetLanguage,
		},
		{
			label: "command", variable: config.CommandVar, kind: text,
			value: settings.Options.Command, empty: "no command set",
		},
		{
			label: "submit", variable: config.SubmitVar, kind: flag,
			value: onOff(settings.Submit),
		},
		{
			label: "live", variable: config.LiveVar, kind: flag,
			value: onOff(settings.Live),
		},
		{
			label: "vim", variable: config.VimVar, kind: flag,
			value: onOff(settings.Vim),
		},
		{
			label: "confirm", variable: config.ConfirmVar, kind: flag,
			value: onOff(settings.Confirm),
		},
		{
			label: "keep draft", variable: config.KeepDraftVar, kind: flag,
			value: onOff(settings.KeepDraft),
		},
		{
			// An empty theme is auto: the palette the window has always drawn
			// with, so it stands first and is shown under that name.
			label: "theme", variable: config.ThemeVar, kind: choice,
			choices: append([]string{"auto"}, frame.ThemeNames()...),
			value:   settings.Theme,
		},
		{
			label: "draft rows", variable: config.DraftRowsVar, kind: number,
			value: strconv.Itoa(settings.DraftRows),
			min:   config.DraftRowsLow, max: config.DraftRowsHigh,
		},
		{
			label: "panel width", variable: config.PanelWidthVar, kind: number,
			value: strconv.Itoa(settings.PanelWidth),
			min:   config.PanelWidthLow, max: config.PanelWidthHigh,
		},
		{
			label: "paste keys", variable: config.PasteVar, kind: chord,
			value: settings.PasteKeys, empty: "automatic",
		},
		{
			label: "select copy", variable: config.SelectCopyVar, kind: chord,
			value: settings.SelectCopy, empty: "clipboard as it stands",
		},
		{
			label: "panel hotkey", variable: config.HotkeyVar, kind: hotkey,
			value: settings.Hotkey,
		},
		{
			label: "selection hotkey", variable: config.SelectHotkeyVar, kind: hotkey,
			value: settings.SelectHotkey,
		},
		{
			label: "settings hotkey", variable: config.ConfigHotkeyVar, kind: hotkey,
			value: settings.ConfigHotkey,
		},
	}

	for _, row := range rows {
		row.original = row.value
		if options.Getenv != nil {
			row.env = strings.TrimSpace(options.Getenv(row.variable)) != ""
		}
	}
	return rows
}

func onOff(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
