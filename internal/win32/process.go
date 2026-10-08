package win32

import "strings"

// Process is one program Windows is running, named by the executable it came
// from. The tree of them �� who is whose parent �� is what says whether a
// terminal is sitting at a prompt or has a program of its own on the screen.
type Process struct {
	// Name is the executable's own name, as Windows reports it: "pwsh.exe".
	Name   string
	PID    uint32
	Parent uint32
}

// shells are the programs that read a command line at a prompt. A command
// typed into a window is read by one of these, or by whatever one of them
// started �� which is exactly what a command must not be typed into.
var shells = map[string]bool{
	"pwsh.exe":       true,
	"powershell.exe": true,
	"cmd.exe":        true,
	"bash.exe":       true,
	"sh.exe":         true,
	"dash.exe":       true,
	"zsh.exe":        true,
	"fish.exe":       true,
	"nu.exe":         true,
	"elvish.exe":     true,
	"xonsh.exe":      true,
	"csh.exe":        true,
	"tcsh.exe":       true,
	"ksh.exe":        true,
	"wsl.exe":        true,
}

// consoleHosts draw a terminal rather than being the shell in it. A terminal
// window belongs to one of these, and the shells of its panes sit beside and
// under it; a host's own process never reads what is typed into the pane.
var consoleHosts = map[string]bool{
	"conhost.exe":                true,
	"openconsole.exe":            true,
	"windowsterminal.exe":        true,
	"windowsterminalpreview.exe": true,
	"wezterm.exe":                true,
	"wezterm-gui.exe":            true,
	"alacritty.exe":              true,
	"hyper.exe":                  true,
	"tabby.exe":                  true,
	"conemu64.exe":               true,
	"conemu.exe":                 true,
	"mintty.exe":                 true,
}

// IsShell says whether this program reads command lines at a prompt.
func IsShell(name string) bool {
	return shells[strings.ToLower(name)]
}

// IsConsoleHost says whether this program draws a terminal window without
// being the shell that reads what is typed into it.
func IsConsoleHost(name string) bool {
	return consoleHosts[strings.ToLower(name)]
}

// windowsTerminals are the Windows Terminal family: the only windows whose
// panes can be split and focused by asking the terminal itself. A panel beside
// an agent is a pane of one of these; every other host takes the panel some
// other way.
var windowsTerminals = map[string]bool{
	"windowsterminal.exe":        true,
	"windowsterminalpreview.exe": true,
	"windowsterminalcanary.exe":  true,
}

// IsWindowsTerminal says whether this program is a Windows Terminal.
func IsWindowsTerminal(name string) bool {
	return windowsTerminals[strings.ToLower(name)]
}

// OnlyShells says whether every process named is a shell, and there is at
// least one: a console nobody is on tells nothing.
func OnlyShells(processes []Process) bool {
	found := false
	for _, process := range processes {
		if IsShell(process.Name) {
			found = true
			continue
		}
		return false
	}
	return found
}

// Leaves are the processes under root that nothing else is running under,
// with the terminal hosts left out: what a program leaves as a leaf is either
// the shell at its prompt or the program the shell started.
//
// The root is one of them when it has no children of its own �� a console host
// is then the only thing the tree shows, and the answer is empty, which is a
// question nothing here can answer rather than a prompt.
func Leaves(processes []Process, root uint32) []Process {
	if root == 0 {
		return nil
	}
	// The tree is walked with the full set of relatives rather than in one
	// pass: a process listed before its parent is reached on the next turn of
	// the walk, and its own children come after that.
	under := relatives(processes, root)
	children := map[uint32]bool{}
	for _, process := range processes {
		if under[process.Parent] {
			children[process.Parent] = true
		}
	}

	leaves := []Process{}
	for _, process := range processes {
		if !under[process.PID] || children[process.PID] {
			continue
		}
		// A terminal host left as a leaf means the tree shows the window and
		// nothing of the shell inside it �� a question this cannot answer.
		if IsConsoleHost(process.Name) {
			continue
		}
		leaves = append(leaves, process)
	}
	return leaves
}

// relatives is every process at or under root: root, and anything whose parent
// chain reaches it. The chain is followed rather than assumed, because a
// process Windows started after its parent is listed before it here some days.
func relatives(processes []Process, root uint32) map[uint32]bool {
	under := map[uint32]bool{root: true}
	for changed := true; changed; {
		changed = false
		for _, process := range processes {
			if under[process.PID] || !under[process.Parent] {
				continue
			}
			under[process.PID] = true
			changed = true
		}
	}
	return under
}

// AtPrompt says whether a command typed into a window whose process tree is
// this one would be read by a shell: every leaf under root is a shell, and
// there is at least one. A program running under a shell �� an agent, an
// editor, a compiler �� is a leaf of its own and answers no.
//
// A tree takes in every pane of a terminal at once, so this is the answer when
// nothing narrower can be asked: a window with a program running in any of its
// panes is left alone.
func AtPrompt(processes []Process, root uint32) bool {
	return OnlyShells(Leaves(processes, root))
}

// IsAncestor says whether one process is an ancestor of the other �� or the
// same process, which is how a program recognises a window of its own.
func IsAncestor(processes []Process, ancestor, descendant uint32) bool {
	if ancestor == 0 || descendant == 0 {
		return false
	}
	// A parent Windows has not kept is not a cycle, and one that is a cycle is
	// not walked forever.
	for steps := 0; descendant != ancestor && steps < 64; steps++ {
		parent := parentOf(processes, descendant)
		if parent == 0 || parent == descendant {
			return false
		}
		descendant = parent
	}
	return descendant == ancestor
}

func parentOf(processes []Process, pid uint32) uint32 {
	for _, process := range processes {
		if process.PID == pid {
			return process.Parent
		}
	}
	return 0
}

// TitlesMatch says whether a console's own title is the one a window is
// showing. A terminal names its window after the pane in front, and a shell
// names its console, so the two are the same words when the window is showing
// that shell's pane �� a comparison that survives the small differences a
// terminal adds: a tab title with the profile in it, a colon, a dash.
func TitlesMatch(window, console string) bool {
	if window == "" || console == "" {
		return false
	}
	haystack := strings.ToLower(invisible(window))
	needle := strings.ToLower(invisible(console))
	if haystack == needle {
		return true
	}
	// One naming the other is enough: a window titled "pwsh: D:\work" shows
	// the console titled "D:\work", and a window titled after its profile
	// shows a console titled after its prompt.
	return strings.Contains(haystack, needle) || strings.Contains(needle, haystack)
}
