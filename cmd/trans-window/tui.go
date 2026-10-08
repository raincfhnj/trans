//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"trans/internal/config"
	"trans/internal/overlay"
	"trans/internal/promptflow"
	"trans/internal/win32"
	"trans/internal/winlog"
	"trans/internal/wintarget"

	tea "github.com/charmbracelet/bubbletea"
)

// tuiOptions is what the terminal asked of the panel drawn in it. The panel in
// a terminal is the same box as the popup: what differs is where it is drawn
// and how it is asked for �� typed into a terminal rather than opened over one.
type tuiOptions struct {
	read    bool
	capture bool
	sending sending
	probe   int
	// inline draws the box in the terminal's own scrollback rather than taking
	// the screen over: what was on the terminal stays on it.
	inline bool
	// beside says the panel is a pane split off from the pane it delivers
	// into: the prompt goes to the pane used last before this one, which is
	// the agent beside it however the terminal laid the two out.
	beside bool
	// target names the window the finished prompt is delivered into. Without
	// one the window the author was working in before this terminal is used.
	target string
}

func parseTUI(arguments []string) (tuiOptions, error) {
	var options tuiOptions

	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--read":
			options.read = true
		case "--capture":
			options.capture = true
		case "--review":
			options.sending = sendingReview
		case "--send":
			options.sending = sendingSend
		case "--inline":
			options.inline = true
		case "--beside":
			options.beside = true
		case "--target":
			if index+1 >= len(arguments) {
				return options, errors.New("--target needs a window handle or a title")
			}
			options.target = arguments[index+1]
			index++
		case "--probe":
			if index+1 >= len(arguments) {
				return options, errors.New("--probe needs a number of milliseconds")
			}
			milliseconds, err := strconv.Atoi(arguments[index+1])
			if err != nil || milliseconds < 1 {
				return options, errors.New("--probe wants a number of milliseconds, not " +
					arguments[index+1])
			}
			options.probe = milliseconds
			index++
		default:
			return options, errors.New("unknown argument " + strconv.Quote(arguments[index]))
		}
	}

	if options.capture && !options.read {
		return options, errors.New("--capture belongs to --read: it starts a read-mode draft")
	}
	return options, nil
}

func (o tuiOptions) review(setting bool) bool {
	return o.sending.review(setting)
}

// runTUI is the panel as a TUI in the terminal it is run in. It is the panel
// it always was �� the draft box, the service, the delivery �� with the
// terminal as its frame instead of a popup over the pane in front.
func runTUI(arguments []string) error {
	options, err := parseTUI(arguments)
	if err != nil {
		return err
	}

	config.Prepare()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	protectKey(&cfg)

	chosen := panelService(&cfg, options.read)

	// The window the prompt is delivered into is not the one being drawn in:
	// this terminal is the author's own, and the window that takes the prompt
	// is the one the author was working in before it �� or, in a pane split
	// off beside the agent, the pane used just before this one.
	window, beside, targetTrouble := tuiTarget(options, &cfg)

	pasteKeys := cfg.PasteKeys
	if pasteKeys == "" {
		pasteKeys = win32.DefaultPasteChord()
	}
	sending, typing := deliveryTargets(window, pasteKeys, beside)

	prefill, prefillTrouble := "", error(nil)
	if options.read {
		if options.capture && beside {
			// The selection is in the pane beside, so that is where the copy
			// chord has to be pressed: the focus goes over and comes back.
			moveBeside(true)
			defer moveBeside(false)
		}
		prefill, prefillTrouble = readSource(systemPorts, window.Handle,
			options.capture, cfg.CaptureKeys)
		winlog.Notef("tui", "read mode: %d characters of source, trouble %v",
			len(prefill), prefillTrouble)
	}

	flow := panelFlow(chosen, sending, typing, options.read)

	// ctrl+c is caught as a signal, so the last word on the draft cannot be a
	// keystroke: whatever is in the box when the terminal goes is kept.
	ctx, stopListening := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopListening()

	cursor := &overlay.CursorPlace{}
	programOptions := []tea.ProgramOption{
		tea.WithContext(ctx),
		tea.WithOutput(&cursorFollower{out: os.Stdout, place: cursor}),
	}
	if !options.inline {
		// The TUI is the whole terminal while it is open: the alternate screen
		// is its own, and what was on the terminal comes back when it closes.
		programOptions = append(programOptions, tea.WithAltScreen())
	}

	program := tea.NewProgram(panelModel(ctx, &cfg, chosen, flow, &panelDrawing{
		read:           options.read,
		review:         options.review(!cfg.Submit),
		prefill:        prefill,
		prefillTrouble: prefillTrouble,
		trouble:        targetTrouble,
		slot:           tuiSlot(options, window),
		width:          cfg.PanelWidth,
		cursor:         cursor,
	}), programOptions...)

	closeAfter(program, options.probe)
	winlog.Notef("tui", "opened in this terminal, service %s, delivering to %#x %q",
		chosen.Name, window.Handle, window.Title)
	return runOverlay(program, "tui", "running the panel")
}

// deliveryTargets is where a finished prompt goes. A window to deliver into
// takes it by paste; without one the prompt is copied and the author pastes it
// wherever it belongs. A delivery that cannot get through falls back to the
// clipboard as well, so a prompt is never simply lost.
//
// Beside delivery goes through the pane the panel was split from: the focus
// crosses over for the paste and comes back when the prompt was sent, since a
// sent prompt leaves the agent working and the author writing the next one. A
// prompt that is only typed in keeps the focus where the last keystroke has to
// be pressed.
func deliveryTargets(window win32.Window, pasteKeys string, beside bool) (sending, typing promptflow.Target) {
	if window.Handle == 0 {
		return clipboardTarget{}, clipboardTarget{}
	}
	rawSending := promptflow.Target(wintarget.NewSending(window.Handle, pasteKeys))
	rawTyping := promptflow.Target(wintarget.NewTyping(window.Handle, pasteKeys))
	if beside {
		rawSending = besideTarget{
			inner: rawSending, returnFocus: true,
			move: win32.MoveFocus, wait: time.Sleep,
		}
		rawTyping = besideTarget{
			inner: rawTyping, returnFocus: false,
			move: win32.MoveFocus, wait: time.Sleep,
		}
	}
	return copiedInstead{rawSending}, copiedInstead{rawTyping}
}

// besideTarget delivers into the pane this one was split from. Windows
// Terminal is asked rather than guessed at: the two panes are each other's
// `previous` however the terminal laid them out.
type besideTarget struct {
	inner promptflow.Target
	// returnFocus brings the keyboard back to the panel once the prompt is in.
	returnFocus bool
	move        func(direction string) error
	wait        func(time.Duration)
}

func (b besideTarget) Insert(ctx context.Context, text string) error {
	if err := b.move("previous"); err != nil {
		return fmt.Errorf("crossing to the pane beside this one: %w", err)
	}
	// The pane is asked for the keyboard and then given the prompt: a paste
	// that arrives while the focus is still crossing lands in this pane.
	b.wait(frontSettle)
	err := b.inner.Insert(ctx, text)
	if b.returnFocus {
		if back := b.move("previous"); back != nil {
			winlog.Notef("tui", "the focus did not come back to the panel: %v", back)
		}
	}
	return err
}

// moveBeside crosses to the pane beside this one and back: the same move the
// delivery makes, for what else has to happen over there �� a capture chord
// pressed into a selection the panel only reads.
var moveBeside = func(over bool) {
	if err := win32.MoveFocus("previous"); err != nil {
		winlog.Notef("tui", "crossing to the pane beside: %v", err)
		return
	}
	time.Sleep(frontSettle)
}

// tuiTarget is the window the finished prompt is delivered into: the one named
// on the command line or in the settings �� then, with nothing named, the pane
// beside this one when the panel was split off from it, and the window the
// author was working in before this terminal otherwise, which is the first
// pane below this program's own. The second answer says whether the delivery
// crosses panes rather than addresses a window.
//
// A window that is named and cannot be found is an answer rather than a
// silence: the trouble travels to the panel, which says it out loud.
func tuiTarget(options tuiOptions, cfg *config.Settings) (win32.Window, bool, error) {
	named := options.target
	if strings.TrimSpace(named) == "" {
		named = cfg.Target
	}
	if strings.TrimSpace(named) != "" {
		window, err := win32.Resolve(win32.ParseTarget(named))
		if err != nil {
			return win32.Window{}, false, fmt.Errorf("the window to deliver into: %w", err)
		}
		return window, false, nil
	}

	if options.beside {
		// The panel is a pane of the window in front, and the prompt belongs
		// in the pane beside it: the window is the one to paste into, and the
		// crossing is the delivery's own.
		window := win32.Foreground()
		if window.Handle == 0 {
			return win32.Window{}, false, errors.New(
				"no window to deliver the prompt into: it will be copied to the clipboard")
		}
		return window, true, nil
	}

	self := win32.SelfPID()
	tree := win32.Processes()
	window := win32.FrontPaneExcept(func(handle uintptr) bool {
		return win32.SelfWindow(handle, self, tree) ||
			win32.IsPanelWindow(win32.Title(handle))
	})
	if window.Handle == 0 {
		return win32.Window{}, false, errors.New(
			"no window to deliver the prompt into: it will be copied to the clipboard")
	}
	return window, false, nil
}

// tuiSlot is the name the kept draft and the history go under: the window the
// prompt is delivered into, by the title the author knows it by. A panel
// beside its own pane delivers into a pane rather than a window, and with no
// window in particular the drafts belong to the terminal itself.
func tuiSlot(options tuiOptions, window win32.Window) string {
	if options.beside {
		return "the pane beside"
	}
	if strings.TrimSpace(window.Title) == "" {
		return "the terminal"
	}
	return window.Title
}
