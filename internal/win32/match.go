package win32

import (
	"regexp"
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
func ParseTarget(setting string) Target {
	trimmed := strings.TrimSpace(setting)
	if handle, err := strconv.ParseUint(trimmed, 0, 64); err == nil && handle != 0 {
		return Target{Handle: uintptr(handle)}
	}
	return Target{Pattern: trimmed}
}
