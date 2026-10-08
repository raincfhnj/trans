//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"trans/internal/win32"
	"trans/internal/winlog"
)

// openInTerminal opens the panel as a TUI in the terminal in front. A Windows
// Terminal is asked to split the pane there and run the panel in the new one
// �� the panel beside whatever was on the screen, agent or shell �� and any
// other terminal is typed into at its prompt: the terminal then starts the
// panel the way it starts anything else.
//
// Nothing is typed into a window that is not at a prompt: a command line lands
// in whatever reads the window, and in a pane holding a program of its own
// that program would take it as its own input. What can be asked of the
// console is asked; what cannot is judged by the programs running under it.
func openInTerminal(options openOptions) error {
	window := win32.FrontPane()
	if window.Handle == 0 {
		return errors.New("no terminal in front to open the panel in: " +
			"the panel opens in the terminal the chord was pressed in")
	}

	// A Windows Terminal keeps the pane tree of its own, so the panel is a
	// pane of it rather than a program typed into one: what runs in the pane
	// beside is then none of this program's business.
	if win32.IsWindowsTerminal(win32.ProcessOfWindow(window.Handle).Name) {
		return openBeside(options)
	}
	return openAtPrompt(window, options)
}

// panelShare is how much of the pane's width the panel takes when it is split
// off beside the one it delivers into: enough to read a sentence of the
// translation in, and no more of the agent's room than that.
const panelShare = "0.35"

// panelPaneTitle is the name the panel's pane is given. It is not one of the
// popup titles on purpose: a window whose pane is in front carries that pane's
// name, and "close panel" must never take a terminal window �� the agent's
// pane included �� for a panel to close.
const panelPaneTitle = "trans-panel-tui"

// openBeside asks Windows Terminal to split the pane in front and run the
// panel in the new one, on the right of the pane it was split from. The panel
// is then a pane of the window the agent is in, which is what "beside" means
// to its delivery.
func openBeside(options openOptions) error {
	program, err := os.Executable()
	if err != nil {
		return err
	}
	arguments := splitArguments(program, options)
	winlog.Notef("panel", "opening beside the pane in front as: wt %s",
		strings.Join(arguments, " "))
	return systemInject.split(arguments)
}

// splitArguments is the Windows Terminal command line that opens the panel
// beside the pane in front. `-w 0` is the window in use, `-V` splits it into
// panes side by side, and the new one �� the panel �� lands on the right.
func splitArguments(program string, options openOptions) []string {
	arguments := []string{
		"-w", "0",
		"split-pane", "-V",
		"--size", panelShare,
		"--title", panelPaneTitle,
		"--suppressApplicationTitle",
		program,
	}
	arguments = append(arguments, tuiArguments(options)...)
	return append(arguments, "--beside")
}

// openAtPrompt types the command line that runs the panel into the terminal in
// front, which starts it the way it starts anything else.
func openAtPrompt(window win32.Window, options openOptions) error {
	shell, ok := systemInject.shellAt(win32.ProcessOfWindow(window.Handle), window.Title)
	if !ok {
		return fmt.Errorf("%q is not sitting at a prompt: nothing was typed into it",
			window.Title)
	}

	program, err := os.Executable()
	if err != nil {
		return err
	}
	command := commandFor(shell.Name, program, tuiArguments(options))
	winlog.Notef("panel", "opening in %#x %q as %s: %s",
		window.Handle, window.Title, shell.Name, command)

	if err := systemInject.activate(window.Handle); err != nil {
		return err
	}
	// The window is asked for the keyboard and then given the command: a press
	// that arrives while the window is still coming forward lands in whatever
	// had the focus before.
	systemInject.wait(frontSettle)
	if err := systemInject.typeText(command); err != nil {
		return fmt.Errorf("typing the command into %q: %w", window.Title, err)
	}
	systemInject.press()
	return nil
}

// tuiArguments is the command line the panel is started with in a terminal:
// the switches the opening asked for, and nothing that would need a word of
// the shell's own quoting. A window named by title travels in TRANS_TARGET
// instead �� the panel reads the settings itself when it starts.
func tuiArguments(options openOptions) []string {
	arguments := []string{"tui"}
	if options.read {
		arguments = append(arguments, "--read")
	}
	if options.capture {
		arguments = append(arguments, "--capture")
	}
	if options.submit {
		arguments = append(arguments, "--send")
	} else {
		arguments = append(arguments, "--review")
	}
	if options.probe != "" {
		arguments = append(arguments, "--probe", options.probe)
	}
	// A handle is one word in any shell and travels as it stands. A title is
	// not: it is dropped here rather than quoted wrongly into one.
	if target := win32.ParseTarget(options.target); target.Handle != 0 {
		arguments = append(arguments, "--target", options.target)
	}
	return arguments
}

// commandFor writes the command line for the shell that will read it. A path
// with a room in it is quoted the way that shell quotes a word; one without is
// written as it stands, which every shell reads the same.
func commandFor(shell, program string, arguments []string) string {
	word := program
	if strings.ContainsAny(program, " \t") {
		switch {
		case strings.EqualFold(shell, "pwsh.exe"), strings.EqualFold(shell, "powershell.exe"):
			word = `& "` + program + `"`
		case strings.EqualFold(shell, "cmd.exe"):
			word = `"` + program + `"`
		default:
			word = "'" + strings.ReplaceAll(program, "'", `'\''`) + "'"
		}
	}
	return strings.Join(append([]string{word}, arguments...), " ")
}

// injectPorts is everything opening in a terminal touches outside this
// program: the window it types into, the typing itself, the pane it asks the
// terminal to split, and the two questions asked of the console before
// anything is typed. A test stands in for all of it, which is how the decision
// is checked without a desktop to type into.
type injectPorts struct {
	activate  func(handle uintptr) error
	typeText  func(text string) error
	press     func()
	wait      func(time.Duration)
	split     func(arguments []string) error
	processes func() []win32.Process
	// mates are the programs on this program's own console, and focused the
	// ones on the console a window is showing �� the two ways a terminal's
	// pane can be asked what is running in it.
	mates   func() ([]win32.Process, bool)
	focused func(windowTitle string, processes []win32.Process) ([]win32.Process, bool)
	self    uint32
}

var systemInject = injectPorts{
	activate:  win32.Activate,
	typeText:  win32.Type,
	press:     win32.Enter,
	wait:      time.Sleep,
	split:     win32.TerminalRun,
	processes: win32.Processes,
	mates:     win32.ConsoleMates,
	focused:   win32.FocusedConsoleMates,
	self:      win32.SelfPID(),
}

// shellAt is the decision: the shell that would read a command typed into this
// window, and whether nothing else in there would read it first. A program
// running in the window �� an agent, an editor �� answers no whatever the
// window is called: what is typed would reach the program and not a prompt.
func (p injectPorts) shellAt(window win32.Process, title string) (win32.Process, bool) {
	tree := p.processes()

	// The window this very program was started from is the surest case: its
	// console is this program's own, and what else is on that console is what
	// will read the command.
	if win32.IsAncestor(tree, window.PID, p.self) {
		if mates, ok := p.mates(); ok {
			return shellAmong(mates, p.self)
		}
		return win32.Process{}, false
	}

	// A terminal this program has nothing to do with is asked about through
	// its shells: a terminal names its window after the pane in front, and a
	// shell names its console, so the two answering to one another is that
	// pane's own console �� and what is on it is what reads the command.
	if mates, ok := p.focused(title, tree); ok {
		return shellAmong(mates, p.self)
	}

	// Nothing could be asked of the console itself. What the window runs is
	// what is left to judge by: every leaf of its tree a shell means the panes
	// are at their prompts, and one program of its own anywhere in it is a
	// reason to type nothing.
	if !win32.AtPrompt(tree, window.PID) {
		return win32.Process{}, false
	}
	for _, leaf := range win32.Leaves(tree, window.PID) {
		if win32.IsShell(leaf.Name) {
			return leaf, true
		}
	}
	return win32.Process{}, false
}

// shellAmong answers the shell on this console that reads command lines, and
// whether every other program on it is one too. What a shell is running is not
// a prompt, and a console with nothing on it says nothing at all.
func shellAmong(mates []win32.Process, self uint32) (win32.Process, bool) {
	quiet := make([]win32.Process, 0, len(mates))
	for _, mate := range mates {
		if mate.PID == self {
			continue
		}
		quiet = append(quiet, mate)
	}
	if !win32.OnlyShells(quiet) {
		return win32.Process{}, false
	}
	for _, mate := range quiet {
		if win32.IsShell(mate.Name) {
			return mate, true
		}
	}
	return win32.Process{}, false
}
