//go:build windows

// Command trans-window is the Windows side of trans. There is no herdr on
// this machine to host a popup, so the panel brings its own window and opens it
// over the pane the prompt is written for; the draft box, the keys, the
// translation and the kept draft are the plugin's, unchanged.
//
//	trans-window                 the panel itself, opened by `open`
//	trans-window open            opens the panel over the window in front
//	trans-window open --review   the same, but only types the prompt in
//	trans-window open --target 0x1a2b3c | "Windows Terminal"
//	trans-window list-windows    what can be opened over, with the handles
//	trans-window translate TEXT  asks the configured service, without a panel
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"trans/internal/config"
	"trans/internal/draft"
	"trans/internal/overlay"
	"trans/internal/promptflow"
	"trans/internal/service"
	"trans/internal/translation"
	"trans/internal/win32"
	"trans/internal/winlog"
	"trans/internal/wintarget"
)

func main() {
	winlog.Note("panel", "started with %q, target %q", os.Args, os.Getenv("HERDR_TRANS_TARGET"))

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "open":
			exit(runOpen(os.Args[2:]))
		case "list-windows":
			exit(listWindows())
		case "translate":
			exit(translate(os.Args[2:]))
		}
		// A command was named, so this is not the panel being opened; without
		// this the panel would run after every one of them.
		return
	}
	exit(runPanel())
}

func exit(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "trans-window:", err)
	os.Exit(1)
}

// runOpen works out which window the panel belongs over and opens it there. A
// window named in the setting wins; without one it is the window the author is
// working in, which is the pane a keybinding would have been pressed in.
func runOpen(arguments []string) error {
	submit := true
	setting := ""
	probe := ""

	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--review":
			submit = false
		case "--target":
			if index+1 >= len(arguments) {
				return fmt.Errorf("--target needs a window handle or a title")
			}
			setting = arguments[index+1]
			index++
		case "--probe":
			// For checking that the panel lands where it belongs without a person
			// sitting in front of it: the panel closes itself again.
			if index+1 >= len(arguments) {
				return fmt.Errorf("--probe needs a number of milliseconds")
			}
			probe = arguments[index+1]
			index++
		default:
			return fmt.Errorf("unknown argument %q", arguments[index])
		}
	}
	if setting == "" {
		setting = os.Getenv("HERDR_TRANS_TARGET")
	}

	var window win32.Window
	var err error
	if strings.TrimSpace(setting) != "" {
		window, err = win32.Resolve(win32.ParseTarget(setting))
	} else {
		window = win32.Foreground()
		if window.Handle == 0 {
			err = fmt.Errorf("no window in front to open the panel over")
		}
	}
	if err != nil {
		return err
	}

	program, err := os.Executable()
	if err != nil {
		return err
	}

	environment := append(os.Environ(),
		fmt.Sprintf("HERDR_TRANS_TARGET=%#x", window.Handle),
		"HERDR_TRANS_TARGET_TITLE="+window.Title,
		"HERDR_TRANS_SUBMIT="+boolSetting(submit),
	)
	if probe != "" {
		environment = append(environment, "HERDR_TRANS_PROBE_MS="+probe)
	}

	// Windows Terminal places a window better than a console window places
	// itself: it knows its own font and is already where the screen is, and the
	// panel does not have to move the window that draws it while that window is
	// still settling — which is what leaves it drawing onto an empty one.
	if terminal := win32.TerminalProgram(); terminal != "" {
		left, top, _, _ := win32.WindowRectOf(window.Handle)
		return win32.SpawnQuietly(terminal, []string{
			"-w", "-1",
			"--pos", fmt.Sprintf("%d,%d", left+panelMargin, top+panelMargin),
			"--size", fmt.Sprintf("%d,%d", overlay.PopupWidth, overlay.PopupHeight()),
			"new-tab", "--title", win32.PanelTitle,
			program,
		}, environment)
	}
	return win32.Spawn(program, nil, environment)
}

// panelMargin keeps the panel off the very edge of the window it opens over.
const panelMargin = 48

// runPanel is the program the popup runs: it puts its own console over the pane
// and hands the draft box the window as its way out.
func runPanel() error {
	defaultDirectories()

	// The window drawing this process is found by the name of its console: with a
	// terminal program as the default host, that is the only handle on it there is.
	win32.NameConsole(win32.PanelTitle)

	settings, err := config.Load(os.Getenv)
	if err != nil {
		winlog.Note("panel", "settings: %v", err)
		return err
	}
	window, err := win32.Resolve(win32.ParseTarget(settings.Target))
	if err != nil {
		winlog.Note("panel", "target %q: %v", settings.Target, err)
		return err
	}

	chosen := service.Choose(&settings)
	translator := chosen.Translator

	pasteKeys := settings.PasteKeys
	if pasteKeys == "" {
		pasteKeys = win32.DefaultPasteChord()
	}

	winlog.Note("panel", "opening over %#x %q, service %s, paste %s",
		window.Handle, window.Title, chosen.Name, pasteKeys)
	if err := win32.OpenOver(win32.PopupOptions{
		Over:   window.Handle,
		Width:  overlay.PopupWidth,
		Height: overlay.PopupHeight(),
	}); err != nil {
		// Not being able to place the window is not a reason to leave: the overlay
		// draws into whatever console this process has, and a panel somewhere
		// beats no panel at all.
		winlog.Note("panel", "the window could not be placed: %v", err)
	}
	targetLeft, targetTop, targetRight, targetBottom := win32.WindowRectOf(window.Handle)
	left, top, right, bottom := win32.WindowRectOf(win32.HostWindow(win32.PanelTitle))
	width, height := win32.ScreenSize()
	winlog.Note("panel", "target %d,%d-%d,%d window %d,%d-%d,%d screen %dx%d",
		targetLeft, targetTop, targetRight, targetBottom, left, top, right, bottom, width, height)

	flowOptions := []promptflow.Option{
		// Writing means translating the same draft again and again, so a preview
		// pays for each sentence once. Protecting sits outside the cache, so a
		// fenced block is taken out before the draft is split into sentences.
		promptflow.WithPreviewTranslator(
			translation.Protecting(translation.Segmented(translator))),
	}
	if spending, keepsCount := service.UsageReporter(translator); keepsCount {
		flowOptions = append(flowOptions, promptflow.WithUsageReporter(spending))
	}

	flow := promptflow.New(
		translation.Protecting(translator),
		wintarget.NewSending(window.Handle, pasteKeys),
		wintarget.NewTyping(window.Handle, pasteKeys),
		flowOptions...,
	)

	// Closing the console window — the panel — ends the program by hanging up on
	// it; ending on the signal instead of dying on it is what keeps the draft.
	ctx, stopListening := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopListening()

	// The caret is put where the writing is in every terminal, Windows Terminal
	// included: the move is written into the panel's own output, which a terminal
	// program takes like any other drawing. An input method's pre-edit text then
	// appears where the writing is instead of at the foot of the panel.
	cursor := &overlay.CursorPlace{}
	programOptions := []tea.ProgramOption{
		tea.WithContext(ctx),
		// The panel is the whole window here, so it may have the alternate screen
		// to itself: nothing of the terminal it was opened over is meant to show.
		tea.WithAltScreen(),
		tea.WithOutput(&cursorFollower{out: os.Stdout, place: cursor}),
	}

	program := tea.NewProgram(
		overlay.New(ctx, flow, overlay.Options{
			Service:        chosen.Name,
			WithoutService: !chosen.Translates,
			Trouble:        chosen.Trouble,
			Language:       settings.Options.TargetLanguage,
			Review:         !settings.Submit,
			Vim:            settings.Vim,
			Live:           settings.Live,
			Confirm:        settings.Confirm,
			Pulse:          settings.Pulse,
			Logo:           settings.Logo,
			MaxDraft:       settings.MaxDraft,
			// Windows Terminal opens alt+enter full screen, so the panel says the key
			// that does send: ctrl+d.
			SendKey: "ctrl+d",
			Cursor:  cursor,

			Drafts: drafts(settings, window.Title),
		}),
		programOptions...,
	)

	// A check that the panel lands where it belongs, with nobody sitting in
	// front of it: the panel closes itself again after the time asked for.
	if probe := os.Getenv("HERDR_TRANS_PROBE_MS"); probe != "" {
		if milliseconds, err := strconv.Atoi(probe); err == nil && milliseconds > 0 {
			go func() {
				time.Sleep(time.Duration(milliseconds) * time.Millisecond)
				program.Quit()
			}()
		}
	}

	final, err := program.Run()
	if kept, ok := final.(overlay.Model); ok {
		if err := kept.KeepUnfinished(); err != nil {
			fmt.Fprintln(os.Stderr, "trans-window:", err)
		}
	}
	winlog.Note("panel", "closed: %v", err)
	// A window that was closed ends the program this way; it is not a failure.
	if err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fmt.Errorf("running the panel: %w", err)
	}
	return nil
}

// cursorFollower keeps the terminal's cursor on the caret, which is where a
// terminal draws the pre-edit text of an input method. Only relative moves are
// used: an absolute row is read against the scrollback a terminal keeps above the
// view, and the panel is then drawn onto an empty one.
type cursorFollower struct {
	out   io.Writer
	place *overlay.CursorPlace
}

func (c cursorFollower) Write(frame []byte) (int, error) {
	written, err := c.out.Write(frame)

	row, column, lines, visible := c.place.Where()
	if !visible || lines <= 0 {
		return written, err
	}
	if up := lines - row; up > 0 {
		_, _ = fmt.Fprintf(c.out, "\x1b[%dA", up)
	}
	if column > 1 {
		_, _ = fmt.Fprintf(c.out, "\x1b[%dC", column-1)
	}
	return written, err
}

func listWindows() error {
	for _, window := range win32.Windows() {
		fmt.Printf("%#x\t%s\n", window.Handle, window.Title)
	}
	return nil
}

func translate(arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("translate needs the text to translate")
	}
	defaultDirectories()
	// Nothing is delivered here, so there is no pane to name; the setting is only
	// what a panel would need.
	if os.Getenv("HERDR_TRANS_TARGET") == "" {
		os.Setenv("HERDR_TRANS_TARGET", "none")
	}

	settings, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	chosen := service.Choose(&settings)
	translated, err := chosen.Translator.Translate(context.Background(), strings.Join(arguments, " "))
	if err != nil {
		return err
	}
	fmt.Printf("[%s] %s\n", chosen.Name, translated)
	return nil
}

// defaultDirectories gives the panel the two directories herdr would have set
// aside for the plugin, so a setting and a draft have a place to live.
func defaultDirectories() {
	if os.Getenv("HERDR_PLUGIN_CONFIG_DIR") == "" {
		if directory, err := os.UserConfigDir(); err == nil {
			os.Setenv("HERDR_PLUGIN_CONFIG_DIR", filepath.Join(directory, "trans"))
		}
	}
	if os.Getenv("HERDR_PLUGIN_STATE_DIR") == "" {
		if directory, err := os.UserCacheDir(); err == nil {
			state := filepath.Join(directory, "trans", "state")
			_ = os.MkdirAll(state, 0o700)
			os.Setenv("HERDR_PLUGIN_STATE_DIR", state)
		}
	}
	writeStarterEnv()
}

// A directory with nothing in it is not much of a start, since the settings are
// files rather than a screen of options.
func writeStarterEnv() {
	directory := os.Getenv("HERDR_PLUGIN_CONFIG_DIR")
	if directory == "" {
		return
	}
	file := filepath.Join(directory, ".env")
	if _, err := os.Stat(file); err == nil {
		return
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return
	}
	starter := "" +
		"# Settings for trans. A line here is a setting; an environment\n" +
		"# variable of the same name wins over it.\n" +
		"#\n" +
		"# Without a key, the free service needs no account at all:\n" +
		"#   HERDR_TRANS_PROVIDER=gtranslate\n" +
		"#\n" +
		"# A key means DeepL, and free keys end in :fx:\n" +
		"#   HERDR_TRANS_API_KEY=your-key\n" +
		"#\n" +
		"# Any OpenAI-compatible API, a gateway or a model on this machine:\n" +
		"#   HERDR_TRANS_PROVIDER=openai\n" +
		"#   HERDR_TRANS_API_KEY=sk-...\n" +
		"#   HERDR_TRANS_ENDPOINT=https://api.deepseek.com/v1\n" +
		"#   HERDR_TRANS_MODEL=deepseek-chat\n"
	_ = os.WriteFile(file, []byte(starter), 0o600)
}

func drafts(settings config.Settings, title string) overlay.Drafts {
	if !settings.KeepDraft || settings.StateDir == "" {
		return nil
	}
	// A handle is different every time a terminal starts; its title is what the
	// author would recognize as the pane they were writing in.
	key := title
	if strings.TrimSpace(key) == "" {
		key = settings.Target
	}
	return draft.NewStore(settings.StateDir).For(key)
}

func boolSetting(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
