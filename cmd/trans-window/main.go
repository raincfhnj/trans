//go:build windows

// Command trans-window is a native Windows translation panel. It opens its own
// window over the target window and delivers the translated prompt via paste.
//
//	trans-window                 the panel itself, opened by `open`
//	trans-window open            opens the panel over the window in front
//	trans-window open --review   the same, but only types the prompt in
//	trans-window open --read     the other way round: English in, your language back
//	trans-window open --read --capture
//	                            read mode, starting from the selection in that window
//	trans-window open --target 0x1a2b3c | "Windows Terminal"
//	trans-window select          reads the selection in the pane in front and
//	                              opens the window that translates it
//	trans-window settings        opens the window that edits the settings
//	trans-window popup [flags]   the window one of the commands above opened,
//	                              in the process of its own: --selection and
//	                              --settings name the other two windows, and
//	                              nothing says the panel itself
//	trans-window list-windows    what can be opened over, with the handles
//	trans-window translate TEXT  asks the configured service, without a panel
//	trans-window setup [--json]  the first-run doctor: what is configured and
//	                              what is missing, with a fix for each line
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"trans/internal/config"
	"trans/internal/draft"
	"trans/internal/history"
	"trans/internal/overlay"
	"trans/internal/promptflow"
	"trans/internal/selection"
	"trans/internal/service"
	"trans/internal/settings"
	"trans/internal/translation"
	"trans/internal/win32"
	"trans/internal/winlog"
	"trans/internal/wintarget"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	winlog.Note("panel", "started with %q, target %q", os.Args, os.Getenv("TRANS_TARGET"))

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "open":
			exit(runOpen(os.Args[2:]))
		case "select":
			exit(runSelect(os.Args[2:]))
		case "settings":
			exit(runSettings(os.Args[2:]))
		case "popup":
			// The window one of the commands above opened, in the process of its
			// own. A person does not type this: what it draws is named on its own
			// command line, which is the only place the child's switches live.
			exit(runPopup(os.Args[2:]))
		case "list-windows":
			listWindows()
		case "translate":
			exit(translate(os.Args[2:]))
		case "setup":
			exit(runSetup(os.Args[2:]))
		default:
			exit(fmt.Errorf("unknown command %q", os.Args[1]))
		}
		// A command was named, so this is not the panel being opened; without
		// this the panel would run after every one of them.
		return
	}
	exit(runPopup(nil))
}

// exit ends the program over a failure. The words go to the standard error,
// which is a console when a person asked for this from one and nowhere at all
// when the daemon opened it; the log is what says it happened either way.
func exit(err error) {
	if err == nil {
		return
	}
	winlog.Note("panel", "gave up: %v", err)
	fmt.Fprintln(os.Stderr, "trans-window:", err)
	os.Exit(1)
}

// openOptions is what the command line asked for. The panel itself is a fresh
// process, so this is also what has to be handed on to it.
type openOptions struct {
	submit  bool
	read    bool
	capture bool
	target  string
	probe   string
}

func parseOpen(arguments []string) (openOptions, error) {
	options := openOptions{submit: true}

	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--review":
			options.submit = false
		case "--read":
			options.read = true
		case "--capture":
			options.capture = true
		case "--target":
			if index+1 >= len(arguments) {
				return options, errors.New("--target needs a window handle or a title")
			}
			options.target = arguments[index+1]
			index++
		case "--probe":
			// For checking that the panel lands where it belongs without a person
			// sitting in front of it: the panel closes itself again.
			if index+1 >= len(arguments) {
				return options, errors.New("--probe needs a number of milliseconds")
			}
			options.probe = arguments[index+1]
			index++
		default:
			return options, fmt.Errorf("unknown argument %q", arguments[index])
		}
	}

	// Capture presses a chord into a window and reads back what it copied. For
	// a panel that delivers into that window the same way it always has, that
	// would be a trick with no purpose — so the two flags belong together.
	if options.capture && !options.read {
		return options, errors.New("--capture belongs to --read: it starts a read-mode draft")
	}
	return options, nil
}

// runOpen works out which window the panel belongs over and opens it there. A
// window named in the setting wins; without one it is the window the author is
// working in, which is the pane a keybinding would have been pressed in.
func runOpen(arguments []string) error {
	options, err := parseOpen(arguments)
	if err != nil {
		return err
	}

	popup := popupOptions{
		read:    options.read,
		capture: options.capture,
		sending: sendingSend,
	}
	if !options.submit {
		popup.sending = sendingReview
	}
	// A probe with nothing readable in it is a caller's mistake rather than a
	// panel that closes itself at once.
	if options.probe != "" {
		milliseconds, err := strconv.Atoi(options.probe)
		if err != nil || milliseconds < 1 {
			return fmt.Errorf("--probe wants a number of milliseconds, not %q", options.probe)
		}
		popup.probe = milliseconds
	}
	return openOver(popup, options.target, overlay.PopupWidth, overlay.PopupHeight())
}

// popupOptions is which window the child process draws, and what it was asked
// for. It travels on the command line rather than in the environment: a switch
// in an environment variable is a switch nothing can check, invisible to
// `trans-window --help`, and read back by a program that has no way of knowing
// who set it.
type popupOptions struct {
	// selection and settings name the two smaller windows; neither means the
	// panel itself.
	selection bool
	settings  bool
	// read is the panel the other way round, and capture starts its draft from
	// the selection in the window it opens over.
	read    bool
	capture bool
	// sending is what the send key does. The command line that opens a panel
	// always says which, so a setting changed while the panel is open cannot
	// reach it; a panel opened with no command line at all follows the setting.
	sending sending
	// probe closes the panel again after this many milliseconds, for checking
	// that it lands where it belongs with nobody sitting in front of it.
	probe int
}

// sending is what the child's send key does.
type sending int

const (
	// sendingPerSetting means the command line said nothing, so TRANS_SUBMIT
	// decides.
	sendingPerSetting sending = iota
	sendingReview
	sendingSend
)

// review answers whether the send key only types the prompt in, given what the
// setting would have said.
func (o popupOptions) review(setting bool) bool {
	switch o.sending {
	case sendingReview:
		return true
	case sendingSend:
		return false
	default:
		return setting
	}
}

// arguments is how this window is asked for: the command line the child is
// started with, which is the whole of what it is told.
func (o popupOptions) arguments() []string {
	arguments := []string{"popup"}
	switch {
	case o.selection:
		arguments = append(arguments, "--selection")
	case o.settings:
		arguments = append(arguments, "--settings")
	}
	if o.read {
		arguments = append(arguments, "--read")
	}
	if o.capture {
		arguments = append(arguments, "--capture")
	}
	switch o.sending {
	case sendingReview:
		arguments = append(arguments, "--review")
	case sendingSend:
		arguments = append(arguments, "--send")
	}
	if o.probe > 0 {
		arguments = append(arguments, "--probe", strconv.Itoa(o.probe))
	}
	return arguments
}

// parsePopup reads the command line of the window a command opened. Every
// combination that cannot be drawn is refused here rather than drawn wrongly:
// there is no panel, no selection and no settings at once.
func parsePopup(arguments []string) (popupOptions, error) {
	var options popupOptions

	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--selection":
			options.selection = true
		case "--settings":
			options.settings = true
		case "--read":
			options.read = true
		case "--capture":
			options.capture = true
		case "--review":
			options.sending = sendingReview
		case "--send":
			options.sending = sendingSend
		case "--probe":
			if index+1 >= len(arguments) {
				return options, errors.New("--probe needs a number of milliseconds")
			}
			milliseconds, err := strconv.Atoi(arguments[index+1])
			if err != nil || milliseconds < 1 {
				return options, fmt.Errorf("--probe wants a number of milliseconds, not %q",
					arguments[index+1])
			}
			options.probe = milliseconds
			index++
		default:
			return options, fmt.Errorf("unknown argument %q", arguments[index])
		}
	}

	switch {
	case options.selection && options.settings:
		return options, errors.New("--selection and --settings name different windows")
	case options.capture && !options.read:
		// Capture presses a chord into a window and reads back what it copied:
		// for a panel that delivers into that window the same way it always
		// has, that would be a trick with no purpose.
		return options, errors.New("--capture belongs to --read: it starts a read-mode draft")
	case (options.selection || options.settings) && (options.read || options.capture):
		return options, errors.New("--read is the panel's own: these windows do not translate a draft")
	}
	return options, nil
}

// popupWindow is the window a popup belongs over: the one named in the
// setting, and — when nothing is named — the pane in front, which is the one
// the chord was pressed in. That pane is not whatever holds the keyboard at
// the moment: the tray’s own hidden window can, and a panel opened over
// that is opened over a box in the corner of the screen rather than over the
// window the author was writing in.
func popupWindow(setting string) (win32.Window, error) {
	if strings.TrimSpace(setting) == "" {
		setting = os.Getenv("TRANS_TARGET")
	}
	if strings.TrimSpace(setting) != "" {
		return win32.Resolve(win32.ParseTarget(setting))
	}
	window := win32.FrontPane()
	if window.Handle == 0 {
		return win32.Window{}, errors.New("no pane in front to open the panel over: " +
			"a panel opens over a visible window large enough to work in")
	}
	return window, nil
}

// openOver opens one of the windows over a pane. What the child draws travels
// on its command line, so a window that was asked for wrongly is refused before
// anything is drawn; the environment carries what is a setting or a piece of
// text rather than a switch — the pane to open over, and the selection one of
// the windows was opened for.
func openOver(options popupOptions, setting string, width, height int, extra ...string) error {
	window, err := popupWindow(setting)
	if err != nil {
		return err
	}
	program, err := os.Executable()
	if err != nil {
		return err
	}

	// The pane is resolved here, while it is still the one the chord was pressed
	// in, and handed on as the target the child opens over.
	environment := append(os.Environ(), fmt.Sprintf("TRANS_TARGET=%#x", window.Handle))
	environment = append(environment, extra...)
	arguments := options.arguments()

	// Windows Terminal places a window better than a console window places
	// itself: it knows its own font and is already where the screen is, and the
	// panel does not have to move the window that draws it while that window is
	// still settling — which is what leaves it drawing onto an empty one.
	if terminal := win32.TerminalProgram(); terminal != "" {
		left, top, _, _ := win32.WindowRectOf(window.Handle)
		tab := []string{
			"-w", "-1",
			"--pos", fmt.Sprintf("%d,%d", left+panelMargin, top+panelMargin),
			"--size", fmt.Sprintf("%d,%d", width, height),
			"new-tab", "--title", win32.PanelTitle,
			program,
		}
		return win32.SpawnQuietly(terminal, append(tab, arguments...), environment)
	}
	return win32.Spawn(program, arguments, environment)
}

// runSelect reads what is selected in the pane in front and opens the window
// that translates it. The reading happens here, in the process the chord woke:
// the clipboard is only right for as long as nobody disturbs it, so the text
// travels on to the window rather than being read there again.
func runSelect(arguments []string) error {
	if len(arguments) > 0 {
		return errors.New("select does not take arguments")
	}
	config.Prepare()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	protectKey(&cfg)

	selected, readErr := win32.SelectedText(cfg.SelectCopy)
	// A selection longer than the service would take is cut to what it would;
	// the window draws its source as far as the box goes anyway.
	if runes := []rune(selected); len(runes) > selectionLimit {
		selected = string(runes[:selectionLimit]) + "…"
	}

	extra := []string{"TRANS_SELECTION=" + selected}
	if readErr != nil {
		extra = append(extra, "TRANS_SELECTION_ERROR="+readErr.Error())
	}
	return openOver(popupOptions{selection: true}, "", selection.PopupWidth,
		selection.PopupHeight(), extra...)
}

// selectionLimit is the longest selection carried on to be translated: long
// enough for a paragraph and its neighbours, short enough that the window and
// the service are both asked something they can answer.
const selectionLimit = 4000

// runSettings opens the window that edits this plugin's settings. Nothing is
// read for it first: the window opens on the settings as they are and writes
// them back through the file itself.
func runSettings(arguments []string) error {
	if len(arguments) > 0 {
		return errors.New("settings does not take arguments")
	}
	config.Prepare()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	protectKey(&cfg)
	options := windowSettings(&cfg)
	return openOver(popupOptions{settings: true}, "", settings.PopupWidth,
		settings.PopupHeight(options))
}

// protectKey moves a plaintext key into the protected store if there is one to
// move, and fills the key back in from there when the file no longer carries
// it. Every program of this tree that has just read the settings does this, so
// the services see the same Options.APIKey whether the key is in the file, in
// the protected store, or in neither.
func protectKey(cfg *config.Settings) {
	if note := config.UpgradeSecrets(cfg); note != "" {
		winlog.Note("window", "%s", note)
	}
	if note := config.ResolveKey(cfg); note != "" {
		winlog.Note("window", "%s", note)
	}
}

// panelMargin keeps the panel off the very edge of the window it opens over.
const panelMargin = 48

// runPopup is the program every popup runs: it puts its own console over the
// pane and draws whichever window its command line named.
func runPopup(arguments []string) error {
	options, err := parsePopup(arguments)
	if err != nil {
		return err
	}

	config.Prepare()

	// The window drawing this process is found by the name of its console: with a
	// terminal program as the default host, that is the only handle on it there is.
	win32.NameConsole(win32.PanelTitle)

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		winlog.Note("panel", "settings: %v", err)
		return err
	}
	// Every window of this program asks a service the same question of the same
	// settings, so the key is resolved once here rather than in the panel alone:
	// one kept in the protected store has to reach all of them.
	protectKey(&cfg)

	window, resolveErr := win32.Resolve(win32.ParseTarget(cfg.Target))
	// The selection and the settings are drawn by the same program as the
	// panel; what they do not have is its draft box and its sending.
	if options.selection || options.settings {
		if resolveErr != nil {
			// These windows only put something on the screen: opening them
			// where they can beats not opening at all because the pane named
			// for them could not be found. The pane in front is where they go
			// then, which is where the chord was pressed.
			winlog.Note("panel", "target %q: %v", cfg.Target, resolveErr)
			window = win32.FrontPane()
		}
		if options.selection {
			return selectionWindow(window, &cfg)
		}
		return settingsWindow(window, &cfg)
	}
	if resolveErr != nil {
		winlog.Note("panel", "target %q: %v", cfg.Target, resolveErr)
		return resolveErr
	}
	return runPanel(&cfg, window, options)
}

// runPanel is the draft box itself: the service a draft is translated with, and
// the pane the finished prompt is delivered into.
func runPanel(cfg *config.Settings, window win32.Window, options popupOptions) error {
	// Read mode is the panel the other way round. The command line that opened
	// it said so; without that this is the panel it always was.
	read := options.read
	if read {
		// The translator is the one that was chosen — same provider, same
		// credentials, same bill — pointed the other way: what comes in is text
		// to read, what comes back is the author's own language.
		cfg.Options.TargetLanguage = cfg.ReadLanguage
	}
	chosen := service.Choose(cfg)

	pasteKeys := cfg.PasteKeys
	if pasteKeys == "" {
		pasteKeys = win32.DefaultPasteChord()
	}

	// The panel is about to cover the window it opens over, so a selection
	// there has to be picked up before that, while the window can still take
	// the keys.
	prefill, prefillTrouble := panelPrefill(cfg, window, options)

	winlog.Note("panel", "opening over %#x %q, service %s, paste %s",
		window.Handle, window.Title, chosen.Name, pasteKeys)
	placeOver(window, overlay.PopupWidth, overlay.PopupHeight())

	flow := panelFlow(chosen, window, pasteKeys, read)

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
			Language:       cfg.Options.TargetLanguage,
			Review:         options.review(!cfg.Submit),
			Vim:            cfg.Vim,
			Live:           cfg.Live,
			Confirm:        cfg.Confirm,
			Pulse:          cfg.Pulse,
			Logo:           cfg.Logo,
			MaxDraft:       cfg.MaxDraft,
			// Windows Terminal opens alt+enter full screen, so the panel says the key
			// that does send: ctrl+d.
			SendKey: "ctrl+d",
			Cursor:  cursor,

			Read:           read,
			Prefill:        prefill,
			PrefillTrouble: prefillTrouble,

			Drafts:  drafts(cfg, window.Title),
			History: historyLog(cfg, window.Title),
		}),
		programOptions...,
	)

	closeAfter(program, options.probe)

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

// panelPrefill is the text a read-mode panel opens on: the selection in the
// pane it covers when a capture was asked for, and the clipboard without one.
// It is read here rather than in the panel because the panel is about to cover
// that pane, and a covered pane cannot be asked what is selected.
func panelPrefill(cfg *config.Settings, window win32.Window, options popupOptions) (string, error) {
	if !options.read {
		return "", nil
	}

	source, trouble := readSource(systemPorts, window.Handle, options.capture, cfg.CaptureKeys)
	winlog.Note("panel", "read mode: %d characters of source, trouble %v", len(source), trouble)
	return source, trouble
}

// panelFlow wires the draft's two ways out: the service it is translated with,
// and the window the finished prompt is delivered into. Writing means
// translating the same draft again and again, so the preview goes through a
// sentence cache that pays for each sentence once; Protecting sits outside it,
// so a fenced block is taken out before the draft is split into sentences.
func panelFlow(chosen service.Choice, window win32.Window,
	pasteKeys string, read bool,
) *promptflow.Flow {
	translator := chosen.Translator
	flowOptions := []promptflow.Option{
		promptflow.WithPreviewTranslator(
			translation.Protecting(translation.Segmented(translator))),
	}
	if spending, keepsCount := service.UsageReporter(translator); keepsCount {
		flowOptions = append(flowOptions, promptflow.WithUsageReporter(spending))
	}

	// There are two ways a prompt reaches an agent and no more, which is why
	// the flow names them both. In read mode neither happens: the translation
	// is copied to the clipboard, and the author pastes it where it belongs.
	var sending, typing promptflow.Target = wintarget.NewSending(window.Handle, pasteKeys),
		wintarget.NewTyping(window.Handle, pasteKeys)
	if read {
		sending, typing = copying{}, copying{}
	}
	return promptflow.New(translation.Protecting(translator), sending, typing, flowOptions...)
}

// closeAfter closes the panel again after the time a probe asked for: a check
// that it lands where it belongs needs nobody sitting in front of it.
func closeAfter(program *tea.Program, milliseconds int) {
	if milliseconds <= 0 {
		return
	}
	go func() {
		time.Sleep(time.Duration(milliseconds) * time.Millisecond)
		program.Quit()
	}()
}

// placeOver puts the console over the window the popup belongs on and says
// where everything landed. Not being able to place it is not a reason to
// leave: the window draws into whatever console this process has, and a panel
// somewhere beats no panel at all, so this only ever reports.
func placeOver(window win32.Window, width, height int) {
	if err := win32.OpenOver(win32.PopupOptions{
		Over:   window.Handle,
		Width:  width,
		Height: height,
	}); err != nil {
		winlog.Note("panel", "the window could not be placed: %v", err)
	}
	targetLeft, targetTop, targetRight, targetBottom := win32.WindowRectOf(window.Handle)
	left, top, right, bottom := win32.WindowRectOf(win32.HostWindow(win32.PanelTitle))
	screenWidth, screenHeight := win32.ScreenSize()
	winlog.Note("panel", "target %d,%d-%d,%d window %d,%d-%d,%d screen %dx%d",
		targetLeft, targetTop, targetRight, targetBottom, left, top, right, bottom,
		screenWidth, screenHeight)
}

// selectionWindow draws the selection and its translation. The text was read
// by the process the chord woke and arrives through the environment, so what
// is left here is the service to ask and the window to ask it in.
func selectionWindow(window win32.Window, cfg *config.Settings) error {
	placeOver(window, selection.PopupWidth, selection.PopupHeight())

	chosen := service.Choose(cfg)
	trouble := chosen.Trouble
	// The selection did not come to hand: that is what the window has to say,
	// rather than the state of a service that was never reached.
	if read := os.Getenv("TRANS_SELECTION_ERROR"); read != "" {
		trouble = errors.New("reading the selection: " + read)
	}

	ctx, stopListening := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopListening()

	// Protecting keeps the selection's own code out of the request: what was
	// selected is often code, and it is not prose to be translated.
	return runSmall(ctx, selection.New(ctx, translation.Protecting(chosen.Translator),
		selection.Options{
			Service:        chosen.Name,
			Language:       cfg.Options.TargetLanguage,
			Source:         os.Getenv("TRANS_SELECTION"),
			SelectCopy:     cfg.SelectCopy,
			Trouble:        trouble,
			WithoutService: !chosen.Translates,
		}))
}

// settingsWindow draws the settings over the pane they were called up in.
// There is no service to ask and nothing to send: the window reads what the
// settings are and writes the file they live in.
func settingsWindow(window win32.Window, cfg *config.Settings) error {
	options := windowSettings(cfg)
	placeOver(window, settings.PopupWidth, settings.PopupHeight(options))

	ctx, stopListening := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopListening()
	return runSmall(ctx, settings.New(options))
}

// windowSettings is what the settings window opens on: these settings as they
// are read in this process, every service the registry knows to step through,
// and the real key parsing for the chords it would write.
func windowSettings(cfg *config.Settings) settings.Options {
	return settings.Options{
		Settings: *cfg,
		Getenv:   os.Getenv,
		Services: service.Registry().Names(),
		Chord: func(spec string) error {
			_, _, err := win32.Keys(spec)
			return err
		},
	}
}

// runSmall is the way the two small windows are shown: the console they open
// in is the whole of them, and one closed by hand is the window ending, not a
// failure.
func runSmall(ctx context.Context, model tea.Model) error {
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithAltScreen())
	if _, err := program.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fmt.Errorf("running the window: %w", err)
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

func listWindows() {
	for _, window := range win32.Windows() {
		fmt.Printf("%#x\t%s\n", window.Handle, window.Title)
	}
}

func translate(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("translate needs the text to translate")
	}
	config.Prepare()
	// Nothing is delivered here, so there is no pane to name; the setting is only
	// what a panel would need.
	if os.Getenv("TRANS_TARGET") == "" {
		_ = os.Setenv("TRANS_TARGET", "none")
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	protectKey(&cfg)
	chosen := service.Choose(&cfg)
	// The command line carries the text the way the panel does — code and all —
	// so it goes out through the same protection: a fenced or backticked span is
	// kept out of the request rather than sent to the service as prose.
	translated, err := translation.Protecting(chosen.Translator).
		Translate(context.Background(), strings.Join(arguments, " "))
	if err != nil {
		return err
	}
	fmt.Printf("[%s] %s\n", chosen.Name, translated)
	return nil
}

func drafts(cfg *config.Settings, title string) overlay.Drafts {
	if !cfg.KeepDraft || cfg.StateDir == "" {
		return nil
	}
	return draft.NewStore(cfg.StateDir).For(windowKey(cfg, title))
}

// historyLog is the record of prompts this panel delivers. Every window writes
// into the same file, each entry saying which window it was delivered into.
func historyLog(cfg *config.Settings, title string) overlay.History {
	if !cfg.History || cfg.StateDir == "" {
		return nil
	}
	return history.NewStore(cfg.StateDir, cfg.HistoryLimit).For(windowKey(cfg, title))
}

// windowKey is how a window is named in the panel's own files. A handle is
// different every time a terminal starts; its title is what the author would
// recognize as the pane they were writing in.
func windowKey(cfg *config.Settings, title string) string {
	if strings.TrimSpace(title) == "" {
		return cfg.Target
	}
	return title
}
