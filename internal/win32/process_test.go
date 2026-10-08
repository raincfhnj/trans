package win32

import "testing"

func TestOnlyShellsTakesEveryProgramBeingOne(t *testing.T) {
	t.Parallel()

	atPrompt := []Process{{Name: "pwsh.exe", PID: 1}, {Name: "cmd.exe", PID: 2}}
	if !OnlyShells(atPrompt) {
		t.Errorf("OnlyShells(%v) = false, want true for shells alone", atPrompt)
	}

	working := []Process{
		{Name: "pwsh.exe", PID: 1},
		{Name: "cmd.exe", PID: 2},
		{Name: "opencode.exe", PID: 3},
	}
	if OnlyShells(working) {
		t.Errorf("OnlyShells(%v) = true, want false with a program running", working)
	}

	if OnlyShells(nil) {
		t.Error("OnlyShells(nil) = true, want false: a console nobody is on says nothing")
	}
}

// A terminal's window belongs to the terminal itself, and the shells of its
// panes sit under it; what the tree has to say is what the leaves hold.
func TestLeavesAreTheShellsAndWhatTheyStarted(t *testing.T) {
	t.Parallel()

	tree := []Process{
		{Name: "WindowsTerminal.exe", PID: 100},
		{Name: "OpenConsole.exe", PID: 101, Parent: 100},
		{Name: "pwsh.exe", PID: 200, Parent: 100},
		{Name: "opencode.exe", PID: 300, Parent: 200},
	}

	got := Leaves(tree, 100)
	if len(got) != 1 || got[0].PID != 300 {
		t.Errorf("Leaves(under the terminal) = %v, want the program the shell started", got)
	}
	if AtPrompt(tree, 100) {
		t.Error("AtPrompt with an agent running = true, want false")
	}

	idle := tree[:3:3]
	if !AtPrompt(idle, 100) {
		t.Errorf("AtPrompt(%v) = false, want true: nothing but the shell is running", idle)
	}
}

// A console host alone in its tree shows the window and nothing of the shell
// in it: the question is unanswered rather than answered with yes.
func TestATerminalAloneInItsTreeSaysNothing(t *testing.T) {
	t.Parallel()

	alone := []Process{{Name: "conhost.exe", PID: 7}}
	if AtPrompt(alone, 7) {
		t.Error("AtPrompt over a console host alone = true, want false: no shell is visible")
	}
	if got := Leaves(alone, 7); len(got) != 0 {
		t.Errorf("Leaves(conhost alone) = %v, want nothing a command could be read by", got)
	}
}

func TestTheRootItselfCountsWhenItIsTheShell(t *testing.T) {
	t.Parallel()

	root := Process{Name: "cmd.exe", PID: 5}
	if !AtPrompt([]Process{root}, 5) {
		t.Error("AtPrompt with the window's own process at a prompt = false, want true")
	}
}

// A process listed before its parent is still its parent's child: the tree is
// walked until nothing new turns up rather than in one pass.
func TestLeavesFindChildrenListedBeforeTheirParent(t *testing.T) {
	t.Parallel()

	outOfOrder := []Process{
		{Name: "go.exe", PID: 9, Parent: 4},
		{Name: "pwsh.exe", PID: 4, Parent: 1},
		{Name: "WindowsTerminal.exe", PID: 1},
	}
	got := Leaves(outOfOrder, 1)
	if len(got) != 1 || got[0].PID != 9 {
		t.Errorf("Leaves(%v) = %v, want the compiler the shell started", outOfOrder, got)
	}
}

func TestIsAncestorFollowsTheChainUpward(t *testing.T) {
	t.Parallel()

	tree := []Process{
		{Name: "WindowsTerminal.exe", PID: 1},
		{Name: "pwsh.exe", PID: 2, Parent: 1},
		{Name: "trans-window.exe", PID: 3, Parent: 2},
	}

	if !IsAncestor(tree, 1, 3) {
		t.Error("the terminal is the program's own ancestor and was not seen as one")
	}
	if !IsAncestor(tree, 3, 3) {
		t.Error("a process is its own ancestor here, for recognising a window of its own")
	}
	if IsAncestor(tree, 3, 1) {
		t.Error("the program does not contain the terminal it runs in")
	}
	if IsAncestor(tree, 0, 3) || IsAncestor(tree, 1, 0) {
		t.Error("a process of id 0 belongs to nobody")
	}

	// A parent Windows has not kept is the end of the walk, and a parent that
	// is the process itself is no walk at all �� neither one loops forever.
	selfParent := []Process{{Name: "a.exe", PID: 1, Parent: 1}}
	if IsAncestor(selfParent, 2, 1) {
		t.Error("IsAncestor took a process for its own parent's child")
	}
	cycle := []Process{{Name: "a.exe", PID: 1, Parent: 2}, {Name: "b.exe", PID: 2, Parent: 1}}
	if IsAncestor(cycle, 5, 1) {
		t.Error("IsAncestor walked a cycle instead of giving up")
	}
}

func TestTitlesMatchOnWhatTheyHaveInCommon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		window  string
		console string
		want    bool
	}{
		{name: "the same words", window: "pwsh", console: "pwsh", want: true},
		{name: "a window with the profile in front", window: "PowerShell: pwsh", console: "pwsh", want: true},
		{name: "the console naming the path", window: "pwsh — D:\\work", console: "D:\\work", want: true},
		{name: "case is nothing", window: "D:\\WORK", console: "d:\\work", want: true},
		{name: "a mark nobody typed", window: "pwsh\u200b", console: "pwsh", want: true},
		{name: "different panes", window: "opencode", console: "pwsh", want: false},
		{name: "nothing said", window: "", console: "pwsh", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := TitlesMatch(test.window, test.console); got != test.want {
				t.Errorf("TitlesMatch(%q, %q) = %v, want %v",
					test.window, test.console, got, test.want)
			}
		})
	}
}

// Only Windows Terminal can be asked to split a pane: the panel beside an
// agent is a pane of one of these, whatever the window is called.
func TestTheWindowsTerminalFamilyIsNamed(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"WindowsTerminal.exe", "WindowsTerminalPreview.exe", "windowsterminalcanary.exe"} {
		if !IsWindowsTerminal(name) {
			t.Errorf("IsWindowsTerminal(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"conhost.exe", "wezterm-gui.exe", "Code.exe", "opencode.exe"} {
		if IsWindowsTerminal(name) {
			t.Errorf("IsWindowsTerminal(%q) = true, want false", name)
		}
	}
}
