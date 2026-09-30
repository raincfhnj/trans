//go:build !windows

// The notification area is a Windows thing; everywhere else there is nothing
// to put an icon on, and the daemon is a Windows program besides.
package tray

import "errors"

// Item is one line of the tray's menu. It exists here so the daemon's own
// list of items compiles on every system the tree is built for.
type Item int

const (
	OpenPanel Item = iota + 1
	ClosePanel
	Settings
	Reload
	StartAtLogon
	Quit
)

// Options is what the daemon hands the tray.
type Options struct {
	Tip            string
	Do             func(Item)
	LogonChecked   func() bool
	LogonAvailable func() bool
}

// ErrUnavailable is what a system without a notification area answers.
var ErrUnavailable = errors.New("tray: no notification area on this system")

// Run answers that there is no tray here rather than pretending to draw one.
func Run(Options) error { return ErrUnavailable }
