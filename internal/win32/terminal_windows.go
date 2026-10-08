//go:build windows

package win32

import (
	"errors"
	"os/exec"
	"time"
)

// The Windows Terminal command line: splitting a pane, moving the focus
// between panes, everything the terminal can be asked to do to itself. What is
// asked of it travels as the arguments of `wt`; this file only runs them.
//
// The alias that carries a command hands it to the terminal window and goes,
// which is what is waited for here. A launcher that will not let go is not
// worth waiting for, so the wait is bounded: the command is on its way either
// way, and a terminal asked to split a pane does not need to be watched.

// terminalWait is how long a terminal command is waited on before it is taken
// to be on its way. It is a bound, not a promise: the window a command was
// aimed at acts on it whenever it does.
const terminalWait = 2 * time.Second

// TerminalRun asks Windows Terminal for one thing and waits, boundedly, for
// the asking to be over. The arguments are `wt`'s own: `split-pane -V ...`,
// `move-focus left`, and so on.
func TerminalRun(arguments []string) error {
	terminal := TerminalProgram()
	if terminal == "" {
		return errors.New("no Windows Terminal to ask")
	}
	// The arguments are this program's own and no shell reads them; the wait
	// below is bounded, which is the reason a context would have had.
	command := exec.Command(terminal, arguments...) //nolint:gosec,noctx // G204: an argument list this program built and a wait bounded below
	if err := command.Start(); err != nil {
		return err
	}

	answered := make(chan error, 1)
	go func() { answered <- command.Wait() }()
	select {
	case err := <-answered:
		return err
	case <-time.After(terminalWait):
		return nil
	}
}

// MoveFocus moves the keyboard to the pane Windows Terminal says: `left` and
// `right` by where the panes sit, `previous` by which one was used last. The
// panel is a pane beside the pane it delivers into, and the two are each
// other's `previous` whatever the layout makes of them.
func MoveFocus(direction string) error {
	return TerminalRun([]string{"-w", "0", "move-focus", direction})
}
