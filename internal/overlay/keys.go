package overlay

import "github.com/charmbracelet/bubbles/key"

// keymap is the panel's vocabulary: every keystroke the popup itself answers to,
// named for what pressing it does. One place for all of them is what makes a key
// that means the same thing in three stages recognisable as one key, and it is
// the shape the bubbles components already use.
//
// The words a person reads are not here: the footer and the header draw them, in
// the short form a line has room for (view.go's keyHints and whatHappens), and a
// second copy of them kept here would be a second copy to keep in step. What is
// here is which keys mean what.
//
// The letters the record and the full-screen reading answer to — the arrows,
// j/k, g/G, enter, delete — are matched inside their own files, where those
// panels are drawn and driven. What is here is what the panel decides, wherever
// it is.
type keymap struct {
	Quit      key.Binding
	History   key.Binding
	Flip      key.Binding
	Escape    key.Binding
	Delivery  key.Binding
	Live      key.Binding
	Translate key.Binding
	Clear     key.Binding
	Send      key.Binding
}

// keys is what the panel answers to. A binding matches a keystroke by the name a
// terminal sends it under, which is how the record and the reading pane already
// matched their own letters, so the whole panel now recognizes keys the same way.
var keys = keymap{
	// Quit is the one key no panel may take for itself.
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
	),
	// History opens the record of prompts that already reached the agent, which
	// is how a prompt sent last week can be sent again.
	History: key.NewBinding(
		key.WithKeys("ctrl+g"),
	),
	// Flip is the whole popup given to the translation, and writing again.
	Flip: key.NewBinding(
		key.WithKeys("tab"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
	),
	// Delivery is whether the prompt is sent or only typed into the agent's
	// input, which is easier to choose once the English can be read.
	Delivery: key.NewBinding(
		key.WithKeys("ctrl+r"),
	),
	// Live translation costs characters, so it can be turned on for a sentence
	// that needs watching and off again.
	Live: key.NewBinding(
		key.WithKeys("ctrl+l"),
	),
	// Translate is one translation asked for now, for a draft live translation
	// was never turned on for, or after one that did not arrive.
	Translate: key.NewBinding(
		key.WithKeys("ctrl+t"),
	),
	// Clear throws the draft away, as ctrl+u clears a line in a shell.
	Clear: key.NewBinding(
		key.WithKeys("ctrl+u"),
	),
	// Sending is deliberate: a bare enter is a new line, ctrl+d sends, and
	// alt+enter sends where the terminal passes it through. The terminal hands
	// each press over on its own, so escaping insert mode and pressing enter
	// stay two messages and cannot become a send.
	//
	// The chords are fixed, and deliberately not built from Options.SendKey:
	// that is the name the footer shows for the key, not a keystroke. A terminal
	// that keeps alt+enter for itself — Windows Terminal opens it full screen —
	// has the panel name ctrl+d instead, and the key a person then presses is
	// the same one they would have pressed for the other name.
	Send: key.NewBinding(
		key.WithKeys("ctrl+d", "alt+enter"),
	),
}
