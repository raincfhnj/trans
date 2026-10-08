//go:build windows

package main

import (
	"strings"
	"testing"

	"trans/internal/win32"
)

// A command line typed into a shell is read by that shell: a path with a room
// in it is quoted the way the shell quotes a word, one without is written as
// it stands, which every shell reads the same.
func TestTheCommandIsWrittenForTheShellThatReadsIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		shell   string
		program string
		want    string
	}{
		{
			name:    "a path with no room in it is written as it stands",
			shell:   "pwsh.exe",
			program: `C:\trans\trans-window.exe`,
			want:    `C:\trans\trans-window.exe tui --send`,
		},
		{
			name:    "powershell runs a quoted path with an ampersand",
			shell:   "pwsh.exe",
			program: `C:\Program Files\trans\trans-window.exe`,
			want:    `& "C:\Program Files\trans\trans-window.exe" tui --send`,
		},
		{
			name:    "windows powershell the same",
			shell:   "powershell.exe",
			program: `C:\Program Files\trans\trans-window.exe`,
			want:    `& "C:\Program Files\trans\trans-window.exe" tui --send`,
		},
		{
			name:    "cmd runs a quoted path as it stands",
			shell:   "cmd.exe",
			program: `C:\Program Files\trans\trans-window.exe`,
			want:    `"C:\Program Files\trans\trans-window.exe" tui --send`,
		},
		{
			name:    "a shell of the unix family takes single quotes",
			shell:   "bash.exe",
			program: `/c/Program Files/trans/trans-window.exe`,
			want:    `'/c/Program Files/trans/trans-window.exe' tui --send`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := commandFor(test.shell, test.program, []string{"tui", "--send"})
			if got != test.want {
				t.Errorf("commandFor(%q, %q) = %q, want %q",
					test.shell, test.program, got, test.want)
			}
		})
	}
}

// What the opening asked for travels on the command line the terminal runs;
// what would need a shell's own quoting does not travel at all.
func TestTheCommandLineCarriesTheSwitchesAndAHandleAndNothingElse(t *testing.T) {
	t.Parallel()

	got := tuiArguments(openOptions{submit: true})
	if strings.Join(got, " ") != "tui --send" {
		t.Errorf("tuiArguments(send) = %v, want tui --send", got)
	}

	got = tuiArguments(openOptions{submit: true, read: true, capture: true})
	if strings.Join(got, " ") != "tui --read --capture --send" {
		t.Errorf("tuiArguments(read, capture) = %v, want the two switches and a send", got)
	}

	got = tuiArguments(openOptions{submit: true, probe: "300"})
	if strings.Join(got, " ") != "tui --send --probe 300" {
		t.Errorf("tuiArguments(probe) = %v, want the probe travelling on", got)
	}

	// A handle is one word in any shell and travels as it stands; a title is
	// not a word this program may quote into a shell it only guesses at.
	got = tuiArguments(openOptions{submit: true, target: "0x1a2b3c"})
	if strings.Join(got, " ") != "tui --send --target 0x1a2b3c" {
		t.Errorf("tuiArguments(handle) = %v, want the handle travelling on", got)
	}

	got = tuiArguments(openOptions{submit: true, target: "Windows Terminal"})
	if strings.Join(got, " ") != "tui --send" {
		t.Errorf("tuiArguments(title) = %v, want the title left for TRANS_TARGET", got)
	}
}

// A Windows Terminal is asked to split the pane in front and run the panel in
// the new one: the command line says which window, how the pane is split, and
// what runs in it.
func TestThePanelIsSplitOffBesideThePaneInFront(t *testing.T) {
	t.Parallel()

	got := strings.Join(splitArguments(`C:\trans\trans-window.exe`, openOptions{submit: true}), " ")
	want := "-w 0 split-pane -V --size 0.35 --title trans-panel-tui " +
		"--suppressApplicationTitle C:\\trans\\trans-window.exe tui --send --beside"
	if got != want {
		t.Errorf("splitArguments = %q, want %q", got, want)
	}

	got = strings.Join(splitArguments(`C:\trans\trans-window.exe`,
		openOptions{read: true, capture: true, probe: "300"}), " ")
	if !strings.Contains(got, "tui --read --capture --review --probe 300 --beside") {
		t.Errorf("splitArguments(read) = %q, want the switches and the beside on", got)
	}
}

// The decision is who reads what is typed into a window: a shell at its
// prompt, and nothing else on the console that would read the words first.
func TestACommandIsTypedOnlyWhereAShellWillReadIt(t *testing.T) {
	t.Parallel()

	const self, terminal, console, shell, agent = 350, 100, 101, 200, 300

	// The tree this program runs in: started by the shell, so the window it
	// draws in is one of this program's own.
	atHand := []win32.Process{
		{Name: "WindowsTerminal.exe", PID: terminal},
		{Name: "OpenConsole.exe", PID: console, Parent: terminal},
		{Name: "pwsh.exe", PID: shell, Parent: terminal},
		{Name: "trans-window.exe", PID: self, Parent: shell},
	}
	// The tree behind a window this program was woken for: the daemon started
	// it, so nothing of this program is in that tree at all.
	afar := []win32.Process{
		{Name: "WindowsTerminal.exe", PID: terminal},
		{Name: "OpenConsole.exe", PID: console, Parent: terminal},
		{Name: "pwsh.exe", PID: shell, Parent: terminal},
		{Name: "opencode.exe", PID: agent, Parent: shell},
	}
	idle := afar[:3:3]

	tests := []struct {
		name     string
		window   win32.Process
		title    string
		mates    []win32.Process
		focused  []win32.Process
		tree     []win32.Process
		wantName string
	}{
		{
			name:     "the terminal this program runs in, at a prompt",
			window:   win32.Process{Name: "pwsh.exe", PID: shell},
			mates:    []win32.Process{{Name: "pwsh.exe", PID: shell}},
			tree:     atHand,
			wantName: "pwsh.exe",
		},
		{
			name:   "the terminal this program runs in, with a program of its own",
			window: win32.Process{Name: "pwsh.exe", PID: shell},
			mates: []win32.Process{
				{Name: "pwsh.exe", PID: shell},
				{Name: "opencode.exe", PID: agent},
			},
			tree: atHand,
		},
		{
			name:     "a terminal behind a window of another program, asked of its console",
			window:   win32.Process{Name: "WindowsTerminal.exe", PID: terminal},
			title:    "pwsh",
			focused:  []win32.Process{{Name: "pwsh.exe", PID: shell}},
			tree:     afar,
			wantName: "pwsh.exe",
		},
		{
			name:   "a terminal showing a pane with an agent in it",
			window: win32.Process{Name: "WindowsTerminal.exe", PID: terminal},
			title:  "opencode",
			focused: []win32.Process{
				{Name: "pwsh.exe", PID: shell},
				{Name: "opencode.exe", PID: agent},
			},
			tree: afar,
		},
		{
			name:     "a terminal nothing could be asked of, with every pane idle",
			window:   win32.Process{Name: "WindowsTerminal.exe", PID: terminal},
			tree:     idle,
			wantName: "pwsh.exe",
		},
		{
			name:   "a terminal nothing could be asked of, with a program running",
			window: win32.Process{Name: "WindowsTerminal.exe", PID: terminal},
			tree:   afar,
		},
		{
			name:   "a window that is not a terminal at all",
			window: win32.Process{Name: "chrome.exe", PID: 400},
			tree: []win32.Process{
				{Name: "chrome.exe", PID: 400},
				{Name: "chrome.exe", PID: 401, Parent: 400},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ports := injectPorts{
				self:      self,
				processes: func() []win32.Process { return test.tree },
				mates: func() ([]win32.Process, bool) {
					if test.mates == nil {
						return nil, false
					}
					return test.mates, true
				},
				focused: func(string, []win32.Process) ([]win32.Process, bool) {
					if test.focused == nil {
						return nil, false
					}
					return test.focused, true
				},
			}

			shell, ok := ports.shellAt(test.window, test.title)
			if test.wantName == "" {
				if ok {
					t.Errorf("shellAt(%q) = %v, want nothing typed into it",
						test.title, shell)
				}
				return
			}
			if !ok {
				t.Fatalf("shellAt(%q) refused, want %s to read the command", test.title, test.wantName)
			}
			if shell.Name != test.wantName {
				t.Errorf("shellAt(%q) = %q, want %q", test.title, shell.Name, test.wantName)
			}
		})
	}
}

// A console of its own is asked first: the shell that started this program and
// whatever that shell is running is exactly what reads what is typed into it.
func TestTheOwnConsoleIsTheSurestQuestion(t *testing.T) {
	t.Parallel()

	const self, shell, agent = 1, 200, 300
	tree := []win32.Process{
		{Name: "pwsh.exe", PID: shell},
		{Name: "trans-window.exe", PID: self, Parent: shell},
		{Name: "opencode.exe", PID: agent, Parent: shell},
	}

	asked := false
	ports := injectPorts{
		self:      self,
		processes: func() []win32.Process { return tree },
		mates: func() ([]win32.Process, bool) {
			asked = true
			return []win32.Process{{Name: "pwsh.exe", PID: shell}}, true
		},
		focused: func(string, []win32.Process) ([]win32.Process, bool) {
			t.Error("the console of this program's own was not taken for the answer")
			return nil, false
		},
	}

	got, ok := ports.shellAt(win32.Process{Name: "pwsh.exe", PID: shell}, "")
	if !ok || got.PID != shell {
		t.Errorf("shellAt on this program's own console = %v, %v, want the shell", got, ok)
	}
	if !asked {
		t.Error("the mates of this program's own console were never asked for")
	}
}
