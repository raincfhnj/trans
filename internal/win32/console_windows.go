//go:build windows

package win32

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// PopupOptions says how much room the panel wants and which window it opens over.
type PopupOptions struct {
	Over   uintptr
	Width  int
	Height int
}

// OpenOver puts the window that draws this process over the window the panel was
// opened for: no frame where that is ours to take off, on top of it, and the
// size the overlay asked for.
//
// A terminal emulator cannot host the panel the way herdr hosted a plugin —
// a herdr popup was a pane of herdr's, and there is no herdr here — so the
// panel brings
// its own window. That window is not always reachable through the console: on a
// machine whose default terminal application is Windows Terminal, a console is
// drawn by a window of that program, and the console API answers with something
// hidden. So the panel names its console first and finds its window by that name.
//
// Neither a console nor a terminal program answers about itself until it has
// settled, and a host may move its window once more after it was told where to
// go; so the place is claimed for a while rather than once.
func OpenOver(options PopupOptions) error {
	over, err := Resolve(Target{Handle: options.Over})
	if err != nil {
		return err
	}
	// A pane is a window a person works in. Opening over anything else — a
	// tray's hidden helper, a tooltip, a window nobody can see — would centre a
	// panel over a box in the corner of the screen and hand it the finished
	// prompt as well.
	if !opensOver(over.Handle) {
		return fmt.Errorf("the window %q is not a pane to open over: "+
			"a panel opens over a visible window large enough to work in", over.Title)
	}
	area := windowRect(over.Handle)

	// The panel needs something to draw into: a process with no console has
	// nowhere for the overlay to go, and that is worth failing on here. The
	// resize asks for it again on every attempt and its failure is only a
	// delay, so this is the one place that says a missing console out loud.
	if _, err := consoleHandle(); err != nil {
		return err
	}

	var trouble error
	for range 15 {
		host := hostWindow()
		if host == 0 {
			trouble = errors.New("the window drawing this console cannot be found")
			time.Sleep(80 * time.Millisecond)
			continue
		}
		// The frame comes off a console's own window only; the window of a
		// terminal program draws itself and would come apart.
		if call(procGetConsoleWindow) == host {
			stripFrame(host)
		}
		if trouble = fitToPane(host, options, area); trouble == nil {
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	if trouble != nil {
		return trouble
	}
	return takeKeyboard(hostWindow())
}

// fitToPane asks the console for the cells the overlay wants — the host resizes
// its window to them — and then centres that window over the pane.
func fitToPane(host uintptr, options PopupOptions, area rect) error {
	columns, rows := options.Width, options.Height
	room := workAreaNear(area)

	// The content decides how large the panel is and the screen is the only
	// thing that ever makes it smaller. Fitting it to the pane it covers  what
	// this used to do  is what left a panel so small its own interface was cut
	// off, and a person dragging the window open to read it. A window that
	// shows fewer cells than the overlay draws is a panel cut off at its edge
	// whatever drew it, so the window is made to show every one of them; a host
	// that keeps a frame round the console answers in whole windows, which is
	// why the size is measured again rather than trusted once.
	for range 8 {
		_ = resizeConsole(columns, rows)
		time.Sleep(40 * time.Millisecond)

		present := windowRect(host)
		if present.width() <= 0 || present.height() <= 0 {
			continue
		}
		shownColumns, shownRows := visibleCells(host, columns, rows)

		// How large the window takes to show every cell the overlay draws: a
		// ratio from the cells it shows now, capped by the room the screen
		// gives and never made too small to see.
		wantWidth := capped(resized(present.width(), int32(shownColumns), int32(columns)),
			room.width(), minWindowWidth)
		wantHeight := capped(resized(present.height(), int32(shownRows), int32(rows)),
			room.height(), minWindowHeight)

		if wantWidth != present.width() || wantHeight != present.height() {
			call(procSetWindowPos, host, hwndTopMost, 0, 0,
				uintptr(max(wantWidth, 1)), uintptr(max(wantHeight, 1)),
				swpNoMove|swpNoActivate)
			continue
		}

		// The window is the size the cells need. When the screen would not hold
		// the whole panel, the window is the screen's and the overlay draws the
		// smaller one that fits, rather than one cut off at the edge.
		if shownColumns != columns || shownRows != rows {
			columns = max(minColumns, shownColumns)
			rows = max(minRows, shownRows)
			continue
		}
		break
	}

	present := windowRect(host)
	if present.width() <= 0 || present.height() <= 0 {
		return errors.New("the window drawing this console has no size")
	}

	// Where the panel goes is decided here and nowhere else: centred over the
	// pane it belongs on and kept inside the room the screen gives it, so that
	// the window is one a person can see and reach whatever the pane turned out
	// to be.
	left, top := placedWithin(present.width(), present.height(), area, room)

	for range 6 {
		call(procSetWindowPos, host, hwndTopMost,
			uintptr(left), uintptr(top), 0, 0,
			swpNoSize|swpNoActivate|swpShowWindow)
		if wentTo(host, left, top, present.width(), present.height()) {
			return nil
		}
		time.Sleep(60 * time.Millisecond)
	}
	return fmt.Errorf("the window would not stay at %d,%d", left, top)
}

// consoleScreenBufferInfo is CONSOLE_SCREEN_BUFFER_INFO: how large the buffer
// is and how much of it the window shows. The widths are the API's, not a
// choice.
type consoleScreenBufferInfo struct {
	Size       coord
	Cursor     coord
	Attributes uint16
	Window     smallRect
	MaxSize    coord
}

// visibleCells is how many cells the window drawing this console shows right
// now. A console that will not say is answered with the size asked for, which
// is the best guess there is; the window is measured again either way.
func visibleCells(handle uintptr, fallbackColumns, fallbackRows int) (columns, rows int) {
	info := consoleScreenBufferInfo{}
	if call(procGetConsoleScreenBuffer, handle, uintptr(unsafe.Pointer(&info))) == 0 {
		return fallbackColumns, fallbackRows
	}
	columns = int(info.Window.Right - info.Window.Left + 1)
	rows = int(info.Window.Bottom - info.Window.Top + 1)
	if columns <= 0 || rows <= 0 {
		return fallbackColumns, fallbackRows
	}
	return columns, rows
}

// monitorInfo is what GetMonitorInfoW fills in about one screen: the whole of
// it and the work area, which is the screen with the task bar taken out.
type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

// workAreaNear is the room the screen gives a window: the work area of the
// monitor this place is on. A window is placed inside it and never over the
// task bar or past the screen's edge — the frame is off a popup, so a window
// stranded outside the screen has nothing left to drag it back with.
func workAreaNear(place rect) rect {
	monitor := call(procMonitorFromRect, uintptr(unsafe.Pointer(&place)), monitorDefaultToNearest)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if monitor != 0 && call(procGetMonitorInfoW, monitor, uintptr(unsafe.Pointer(&info))) != 0 {
		return info.Work
	}
	// Nothing answers for a monitor: the primary screen is where a window goes
	// then, which is at least a screen.
	return rect{Right: int32(call(procGetSystemMetrics, 0)), Bottom: int32(call(procGetSystemMetrics, 1))}
}

// wentTo says whether a window is where it was put, give or take the rounding a
// window manager does when it turns pixels back into cells.
func wentTo(handle uintptr, left, top, width, height int32) bool {
	area := windowRect(handle)
	near := func(a, b int32) bool {
		difference := a - b
		return difference > -8 && difference < 8
	}
	return near(area.Left, left) && near(area.Top, top) &&
		near(area.width(), width) && near(area.height(), height)
}

func takeKeyboard(host uintptr) error {
	if host == 0 {
		return errors.New("the window drawing this console cannot be found")
	}
	// Keep the window in the topmost band as well as in front: a popup that
	// merely came to the front falls behind again the moment it loses the
	// keyboard, and a panel a person can no longer see is a panel they drag
	// open to read. Topmost is what stays above the pane it covers whether or
	// not it is the one being typed into. It is claimed again here and once
	// more after the keyboard is taken, so the window is at the top of that
	// band whatever another popup was raised in between.
	call(procSetWindowPos, host, hwndTopMost, 0, 0, 0, 0,
		swpNoMove|swpNoSize|swpNoActivate)
	// Bring it to the front and give it the keyboard the reliable way. A
	// freshly spawned popup is not the foreground process, so a bare
	// SetForegroundWindow is refused more often than not and the panel is left
	// open behind the pane it was meant to cover; Activate borrows the thread
	// that holds the foreground for the moment it takes.
	if err := Activate(host); err != nil {
		return err
	}
	call(procSetWindowPos, host, hwndTopMost, 0, 0, 0, 0,
		swpNoMove|swpNoSize|swpNoActivate)
	return nil
}

// WindowRectOf is where a window is, which is what a check asks about.
func WindowRectOf(handle uintptr) (left, top, right, bottom int32) {
	area := windowRect(handle)
	return area.Left, area.Top, area.Right, area.Bottom
}

// ScreenSize is the coordinate space a window is placed in, which is worth
// knowing when a window does not end up where it was put.
func ScreenSize() (width, height int32) {
	return int32(call(procGetSystemMetrics, 0)), int32(call(procGetSystemMetrics, 1))
}

// TerminalProgram is the terminal that Windows would hand a new console to,
// when it is one that can be asked to open a window of its own. An empty answer
// means there is none, and the console host draws it.
func TerminalProgram() string {
	for _, candidate := range []string{"wt.exe"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}

// ownTitle is the name this process gave its console. A window is looked up by
// it, so the three windows of this program each find the one drawing them
// rather than whichever of them is listed first.
var ownTitle = PanelTitle

// NameConsole gives this console the name the panel looks its window up by, and
// remembers it: the window that draws this process answers to that name alone.
func NameConsole(title string) {
	ownTitle = title
	pointer, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	call(procSetConsoleTitle, uintptr(unsafe.Pointer(pointer)))
}

// OwnTitle is the name this process's window goes by, which is the one it looks
// its own window up under.
func OwnTitle() string { return ownTitle }

// hostWindow is the window that draws this console. A console the console host
// draws has a window of its own, which is the one to place and the one to take
// the frame off; a terminal program's console has none, and the window is found
// by the name this process gave it.
func hostWindow() uintptr {
	if own := call(procGetConsoleWindow); own != 0 {
		area := windowRect(own)
		if area.width() >= minPaneWidth && area.height() >= minPaneHeight {
			return own
		}
	}
	return HostWindow(ownTitle)
}

// HostWindow is the visible window whose title is exactly this, which for a
// console is the window its host draws. A title is not unique on Windows — the
// console host keeps a small window of its own under the same name — so a
// window too small to draw a panel in is passed over.
func HostWindow(title string) uintptr {
	found := uintptr(0)

	callback := syscall.NewCallback(func(handle uintptr, _ uintptr) uintptr {
		if found != 0 || call(procIsWindowVisible, handle) == 0 {
			return 1
		}
		area := windowRect(handle)
		if area.width() < minPaneWidth || area.height() < minPaneHeight {
			return 1
		}
		length := int32(call(procGetWindowTextLengthW, handle))
		if length <= 0 {
			return 1
		}
		buffer := make([]uint16, length+1)
		if call(procGetWindowTextW, handle, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer))) == 0 {
			return 1
		}
		if syscall.UTF16ToString(buffer) == title {
			found = handle
			return 0
		}
		return 1
	})

	call(procEnumWindows, callback, 0)
	return found
}

// stripFrame takes the caption, the border and the scroll bars off a console's
// own window, so what is left is the panel itself.
func stripFrame(console uintptr) {
	style := call(procGetWindowLongPtr, console, gwlStyle)
	style &^= wsCaption | wsThickFrame | wsMinimize | wsMaximize | wsSysMenu |
		wsVScroll | wsHScroll | wsBorder | wsDlgFrame
	style |= wsPopup | wsVisible
	call(procSetWindowLongPtr, console, gwlStyle, style)

	exStyle := call(procGetWindowLongPtr, console, gwlExStyle)
	exStyle &^= wsExWindowEdge | wsExClientEdge | wsExDlgModal | wsExAppWindow
	call(procSetWindowLongPtr, console, gwlExStyle, exStyle)
}

// resizeConsole gives the overlay the cells it asked for. A console changes size
// in this order and no other: the window first, then the buffer it looks into.
// A console that has just been created does not always accept this at once, so
// it is asked again rather than given up on.
func resizeConsole(columns, rows int) error {
	handle, err := consoleHandle()
	if err != nil {
		return err
	}

	var trouble error
	for range 10 {
		if trouble = resizeOnce(handle, columns, rows); trouble == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return trouble
}

func resizeOnce(handle uintptr, columns, rows int) error {
	tiny := smallRect{Left: 0, Top: 0, Right: 0, Bottom: 0}
	if shaped, why := callWhy(procSetConsoleWindowInfo, handle, 1, uintptr(unsafe.Pointer(&tiny))); shaped == 0 {
		return lastError("SetConsoleWindowInfo", why)
	}

	size := coord{X: int16(columns), Y: int16(rows)}
	if shaped, why := callWhy(procSetConsoleScreenBuffer, handle, uintptr(unsafe.Pointer(&size))); shaped == 0 {
		return lastError("SetConsoleScreenBufferSize", why)
	}

	full := smallRect{Left: 0, Top: 0, Right: int16(columns - 1), Bottom: int16(rows - 1)}
	if shaped, why := callWhy(procSetConsoleWindowInfo, handle, 1, uintptr(unsafe.Pointer(&full))); shaped == 0 {
		return lastError("SetConsoleWindowInfo", why)
	}
	return nil
}

// consoleHandle is the buffer the overlay draws into.
func consoleHandle() (uintptr, error) {
	handle := call(procGetStdHandle, stdOutputHandle)
	if handle == 0 || handle == ^uintptr(0) {
		return 0, errors.New("this process has no console output")
	}
	return handle, nil
}

func windowRect(handle uintptr) rect {
	area := rect{}
	call(procGetWindowRect, handle, uintptr(unsafe.Pointer(&area)))
	return area
}

// Spawn opens a program in a console window of its own, which is what lets the
// panel appear where the pane is rather than inside the terminal that opened it.
func Spawn(program string, arguments, environment []string) error {
	return spawn(program, arguments, environment, createNewConsole)
}

// SpawnQuietly opens a program with no window at all, for the step in between
// that only works out where the panel belongs.
func SpawnQuietly(program string, arguments, environment []string) error {
	return spawn(program, arguments, environment, createNoWindow)
}

// The environment block is UTF-16, and a block that is not said to be one is
// read as ANSI — which Windows refuses rather than mishandles.
func spawn(program string, arguments, environment []string, flags uint32) error {
	flags |= createUnicodeEnvironment

	executable, err := syscall.UTF16PtrFromString(program)
	if err != nil {
		return err
	}
	commandLine, err := syscall.UTF16PtrFromString(commandLineFor(program, arguments))
	if err != nil {
		return err
	}

	startup := syscall.StartupInfo{Cb: uint32(unsafe.Sizeof(syscall.StartupInfo{}))}
	var process syscall.ProcessInformation

	if err := syscall.CreateProcess(
		executable,
		commandLine,
		nil, nil, false,
		flags,
		environmentBlock(environment),
		nil,
		&startup,
		&process,
	); err != nil {
		return fmt.Errorf("starting %s: %w", program, err)
	}
	call(procCloseHandle, uintptr(process.Process))
	call(procCloseHandle, uintptr(process.Thread))
	return nil
}

// environmentBlock is what Windows wants to be handed: sorted, without a name
// twice, and ending in an empty string. An unsorted block is refused outright.
func environmentBlock(values []string) *uint16 {
	latest := map[string]string{}
	var names []string

	for _, value := range values {
		name, _, found := strings.Cut(value, "=")
		if !found {
			continue
		}
		key := strings.ToUpper(name)
		if _, known := latest[key]; !known {
			names = append(names, key)
		}
		latest[key] = value
	}
	sort.Strings(names)

	var block []uint16
	for _, name := range names {
		entry, err := syscall.UTF16FromString(latest[name])
		if err != nil {
			// A NUL inside the entry cannot be passed to CreateProcess at
			// all, so the entry is left out of the block.
			continue
		}
		// UTF16FromString ends each entry with the NUL that ends the string;
		// a second one would end the block right there.
		block = append(block, entry...)
	}
	block = append(block, 0)
	return &block[0]
}

// commandLineFor quotes what needs quoting: everything with a space, a tab or a
// quote in it — a program under `Program Files` first of all.
//
// The quoting is not "put quotes round it". The line is parsed back by the
// reader on the other side, where a backslash only means anything before a
// quote: the ones standing before one are doubled and so is the run at the very
// end, or the closing quote is read as an escaped one, the argument never ends,
// and it swallows whatever follows. That rule is the standard library's to
// keep — it is the same escaping os/exec builds a Windows command line with —
// so it is used rather than written again here, and the test keeps the shapes
// that used to break.
func commandLineFor(program string, arguments []string) string {
	parts := append([]string{program}, arguments...)
	for index, part := range parts {
		parts[index] = syscall.EscapeArg(part)
	}
	return strings.Join(parts, " ")
}

// RegisterHotkey claims a key combination for this program under an id, so one
// daemon can wait for the panel's chord, the selection's and the settings
// window's at the same time and say which was pressed. The id travels with the
// key press and no id is claimed twice.
func RegisterHotkey(id, modifiers, key uint32) error {
	if claimed, why := callWhy(procRegisterHotKey, 0, uintptr(id), uintptr(modifiers|modNoRepeat), uintptr(key)); claimed == 0 {
		return lastError("RegisterHotKey", why)
	}
	return nil
}

// ClosePanels asks every window the panel gave itself a name to close, and
// answers the windows that were asked. There is no panel process to toggle —
// one is spawned for each press and ends when its window does — so "close the
// panel" is this: find the windows that said what they are and tell them to go.
// Only windows that carry the name as it stands are asked: a document of the
// author's that merely starts with it is not this program's to close.
//
// The windows come back rather than a count because this is a destructive
// operation on windows that merely look like ours: the caller writes down
// each handle and title it asked to close, so a close that reached the wrong
// window is traceable in the log afterwards instead of being one number.
func ClosePanels() []Window {
	var asked []Window
	for _, window := range Windows() {
		if !IsPanelWindow(window.Title) {
			continue
		}
		if call(procPostMessageW, window.Handle, wmClose, 0, 0) != 0 {
			asked = append(asked, window)
		}
	}
	return asked
}

// UnregisterHotkey lets a claimed combination go, so the daemon can claim a
// new one without restarting. The hotkeys belong to the thread that claimed
// them, so this is called from the loop that waits for them.
func UnregisterHotkey(id uint32) error {
	if freed, why := callWhy(procUnregisterHotKey, 0, uintptr(id)); freed == 0 {
		return lastError("UnregisterHotKey", why)
	}
	return nil
}

// CurrentThreadID names the thread a message queue belongs to. The daemon
// hands it to the tray, which is on a thread of its own and can only ask this
// thread to do something by posting to it.
func CurrentThreadID() uint32 {
	return uint32(call(procGetCurrentThreadId))
}

// Command is what the waiting loop was woken for.
type Command int

const (
	// CommandHotkey is a claimed combination that was pressed; the id says
	// which one.
	CommandHotkey Command = iota
	// CommandReload asks the loop to read the settings and claim the chords
	// again — what the tray's reload item does.
	CommandReload
	// CommandQuit asks the program to end, what closing the window does.
	CommandQuit
)

// WaitForCommand blocks until something the daemon cares about arrives: a
// claimed hotkey, a reload asked for by another thread, or the end of the
// program. Only messages this program posts to itself reach it.
func WaitForCommand() (command Command, id uint32) {
	message := message{}
	for {
		result := call(procGetMessageW, uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		switch {
		case result == 0, result == ^uintptr(0):
			return CommandQuit, 0
		case message.Message == wmHotkey:
			return CommandHotkey, uint32(message.WParam)
		case message.Message == wmReload:
			return CommandReload, 0
		default:
			// Not one of this program's own. Nothing else is waiting on this
			// thread's queue, so a message kept here would be kept forever:
			// the tray runs a loop of its own, but a window created on this
			// thread would have none. Handing it on is harmless when there is
			// no window to hand it to, which is the usual case, and is the
			// whole difference when there is one.
			call(procTranslateMessage, uintptr(unsafe.Pointer(&message)))
			call(procDispatchMessageW, uintptr(unsafe.Pointer(&message)))
		}
	}
}

// PostReload asks the thread that owns the hotkeys to read the settings and
// claim them again. RegisterHotKey is thread-affine — a chord cannot be
// released from anywhere but the thread that claimed it — so the tray only
// sends the request.
func PostReload(thread uint32) error {
	return postThreadMessage(thread, wmReload)
}

// PostQuit asks a thread's message loop to end, which is how one window ends
// a program whose other half is a loop somewhere else.
func PostQuit(thread uint32) error {
	return postThreadMessage(thread, wmQuit)
}

func postThreadMessage(thread, kind uint32) error {
	if sent, why := callWhy(procPostThreadMessage, uintptr(thread), uintptr(kind), 0, 0); sent == 0 {
		return lastError("PostThreadMessage", why)
	}
	return nil
}

// Keys translates a hotkey written the way a person writes one. The letters and
// digits are their own virtual key on Windows, which is all the panel needs.
//
// F12 is refused rather than parsed. Windows keeps it for the debugger and for
// nothing else, and a chord written with it does not merely fail to register:
// the key press goes to whatever has the keyboard, which is not what a setting
// asking to be told about it meant.
func Keys(spec string) (modifiers, key uint32, err error) {
	for part := range strings.SplitSeq(strings.ToLower(spec), "+") {
		switch strings.TrimSpace(part) {
		case "ctrl", "control":
			modifiers |= modControl
		case "alt":
			modifiers |= modAlt
		case "shift":
			modifiers |= modShift
		case "win", "super":
			modifiers |= modWin
		case "f12":
			return 0, 0, errors.New("f12 is reserved by Windows and cannot be used as a hotkey")
		case "":
			continue
		default:
			letter := strings.TrimSpace(part)
			if len(letter) != 1 {
				return 0, 0, fmt.Errorf("%q is not a key", part)
			}
			switch {
			case letter[0] >= 'a' && letter[0] <= 'z':
				key = uint32(letter[0] - 'a' + 'A')
			case letter[0] >= '0' && letter[0] <= '9':
				key = uint32(letter[0])
			default:
				return 0, 0, fmt.Errorf("%q is not a key", part)
			}
		}
	}
	if key == 0 {
		return 0, 0, fmt.Errorf("%q names no key of its own", spec)
	}
	return modifiers, key, nil
}

const (
	// The panel is not worth opening into fewer cells than this, whatever the
	// pane it opens over is.
	minColumns = 40
	minRows    = 8

	// And never into a window too small to read a panel in, whatever the room
	// the screen turns out to give it.
	minWindowWidth  = 320
	minWindowHeight = 160
)
