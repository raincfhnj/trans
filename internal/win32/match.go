package win32

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Window is one top-level window, named the way the task bar names it. Nothing
// here talks to Windows: a list of these is what the Win32 calls produce, and
// what is made of it is plain text.
type Window struct {
	Handle uintptr
	Title  string
}

// PanelTitle is what the panel calls its console, so that it can find the
// window drawing it without caring which program draws it. The selection and
// the settings give themselves names of their own: three windows of this
// program, drawn over a pane and closed together. Each keeps its own name so
// that a window looks up the one drawing *it* — two windows answering to one
// name is how a popup came to be placed over, or raised above, the wrong one.
const (
	PanelTitle    = "trans-panel"
	SelectTitle   = "trans-select"
	SettingsTitle = "trans-settings"
)

// popupTitles are the names this program's windows carry, as they stand.
var popupTitles = []string{PanelTitle, SelectTitle, SettingsTitle}

// PopupTitle is the name a popup gives the window it draws: the selection's and
// the settings' own, and the panel's for everything else.
func PopupTitle(selection, settings bool) string {
	switch {
	case selection:
		return SelectTitle
	case settings:
		return SettingsTitle
	default:
		return PanelTitle
	}
}

// IsPanelWindow says whether a window is one this program drew — the panel, the
// selection or the settings. The name is matched as it stands: a window that
// merely begins with one is a document of the author's — `trans-panel.md` open
// in an editor — and closing one of those is not what "close the panel" means.
func IsPanelWindow(title string) bool {
	return slices.Contains(popupTitles, title)
}

// Find picks the window a target setting names. The title as written is looked
// for first, because that is what someone who copied a title means; a setting
// that matches nothing that way is taken as a regular expression, which is what
// a setting naming several windows looks like.
func Find(windows []Window, pattern string) (Window, bool) {
	if strings.TrimSpace(pattern) == "" {
		return Window{}, false
	}

	lowered := strings.ToLower(invisible(pattern))
	for _, window := range windows {
		if strings.Contains(strings.ToLower(invisible(window.Title)), lowered) {
			return window, true
		}
	}

	expression, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return Window{}, false
	}
	for _, window := range windows {
		if expression.MatchString(window.Title) {
			return window, true
		}
	}
	return Window{}, false
}

// A title carries characters nobody types: a window manager puts a zero-width
// space between words, or a directional mark in front of a right-to-left name.
// What is invisible is taken out before a title is compared.
var invisibleReplacer = strings.NewReplacer(
	"\u200b", "", "\u200c", "", "\u200d", "", "\u200e", "", "\u200f", "", "\ufeff", "",
)

func invisible(text string) string {
	return invisibleReplacer.Replace(text)
}

// Target is what the popup was told to deliver into: either the handle of the
// window the key was pressed in, or a title to look for. A handle is written the
// way Windows prints it, so it can be read out of a listing and pasted back.
type Target struct {
	Handle  uintptr
	Pattern string
}

// ParseTarget reads a target setting. A handle on its own — `0x1a2b` — names the
// window exactly; anything else is a title or a pattern to match one by.
//
// Only the form Windows prints a handle in is read as one. A title that happens
// to be all digits — a document numbered `1024`, a window named `0755` — is a
// title, and reading it as a handle would answer "that window is gone" instead
// of finding it, which is the one thing this setting is for.
func ParseTarget(setting string) Target {
	trimmed := strings.TrimSpace(setting)
	if strings.HasPrefix(strings.ToLower(trimmed), "0x") {
		if handle, err := strconv.ParseUint(trimmed[2:], 16, 64); err == nil && handle != 0 {
			return Target{Handle: uintptr(handle)}
		}
	}
	return Target{Pattern: trimmed}
}
