//go:build windows

package win32

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// PanelTitle is what the panel calls its console, so that it can find the window
// drawing it without caring which program draws it.
const PanelTitle = "trans-panel"

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
// A terminal emulator cannot host the panel the way herdr hosts it — a herdr
// popup is a pane of herdr's, and there is no herdr here — so the panel brings
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
	area := windowRect(over.Handle)
	if area.width() <= 0 || area.height() <= 0 {
		return fmt.Errorf("the window %q has no area to open over", over.Title)
	}

	console, err := consoleHandle()
	if err != nil {
		return err
	}

	var trouble error
	for attempt := 0; attempt < 15; attempt++ {
		host := hostWindow()
		if host == 0 {
			trouble = fmt.Errorf("the window drawing this console cannot be found")
			time.Sleep(80 * time.Millisecond)
			continue
		}
		// The frame comes off a console's own window only; the window of a
		// terminal program draws itself and would come apart.
		if call(procGetConsoleWindow) == host {
			stripFrame(host)
		}
		if trouble = fitToPane(console, host, options, area); trouble == nil {
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
func fitToPane(console, host uintptr, options PopupOptions, area rect) error {
	columns, rows := options.Width, options.Height

	// A window larger than the pane it is meant to sit on is worth asking smaller
	// for; how large a cell is in pixels is the host's business, not ours.
	for attempt := 0; attempt < 3; attempt++ {
		_ = resizeConsole(columns, rows)
		time.Sleep(40 * time.Millisecond)

		present := windowRect(host)
		if present.width() <= area.width() && present.height() <= area.height() {
			break
		}
		if present.width() > area.width() && present.width() > 0 {
			columns = max(minColumns, columns*int(area.width())/int(present.width()))
		}
		if present.height() > area.height() && present.height() > 0 {
			rows = max(minRows, rows*int(area.height())/int(present.height()))
		}
	}

	present := windowRect(host)
	if present.width() <= 0 || present.height() <= 0 {
		return fmt.Errorf("the window drawing this console has no size")
	}

	left := area.Left + (area.width()-present.width())/2
	top := area.Top + (area.height()-present.height())/2

	for attempt := 0; attempt < 6; attempt++ {
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
		return fmt.Errorf("the window drawing this console cannot be found")
	}
	if call(procSetForegroundWindow, host) == 0 {
		return fmt.Errorf("the panel could not take the keyboard")
	}
	return nil
}

// WindowRectOf is where a window is, which is what a check asks about.
func WindowRectOf(handle uintptr) (int32, int32, int32, int32) {
	area := windowRect(handle)
	return area.Left, area.Top, area.Right, area.Bottom
}

// ScreenSize is the coordinate space a window is placed in, which is worth
// knowing when a window does not end up where it was put.
func ScreenSize() (int32, int32) {
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

// NameConsole gives this console the name the panel looks its window up by.
func NameConsole(title string) {
	pointer, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	call(procSetConsoleTitle, uintptr(unsafe.Pointer(pointer)))
}

// OwnConsole is whether the console of this process is drawn by the console host
// itself rather than by a terminal program that keeps the screen to itself. A
// console of its own can be moved, written to and have its cursor placed; one a
// terminal program draws answers to nobody but that program.
func OwnConsole() bool {
	own := call(procGetConsoleWindow)
	if own == 0 {
		return false
	}
	area := windowRect(own)
	return area.width() >= 200 && area.height() >= 120
}

// hostWindow is the window that draws this console. A console the console host
// draws has a window of its own, which is the one to place and the one to take
// the frame off; a terminal program's console has none, and the window is found
// by the name this process gave it.
func hostWindow() uintptr {
	if own := call(procGetConsoleWindow); own != 0 {
		area := windowRect(own)
		if area.width() >= 200 && area.height() >= 120 {
			return own
		}
	}
	return HostWindow(PanelTitle)
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
		if area.width() < 200 || area.height() < 120 {
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
	for attempt := 0; attempt < 10; attempt++ {
		if trouble = resizeOnce(handle, columns, rows); trouble == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return trouble
}

func resizeOnce(handle uintptr, columns, rows int) error {
	tiny := smallRect{Left: 0, Top: 0, Right: 0, Bottom: 0}
	if call(procSetConsoleWindowInfo, handle, 1, uintptr(unsafe.Pointer(&tiny))) == 0 {
		return lastError("SetConsoleWindowInfo")
	}

	size := coord{X: int16(columns), Y: int16(rows)}
	if call(procSetConsoleScreenBuffer, handle, uintptr(unsafe.Pointer(&size))) == 0 {
		return lastError("SetConsoleScreenBufferSize")
	}

	full := smallRect{Left: 0, Top: 0, Right: int16(columns - 1), Bottom: int16(rows - 1)}
	if call(procSetConsoleWindowInfo, handle, 1, uintptr(unsafe.Pointer(&full))) == 0 {
		return lastError("SetConsoleWindowInfo")
	}
	return nil
}

// consoleHandle is the buffer the overlay draws into.
func consoleHandle() (uintptr, error) {
	handle := call(procGetStdHandle, stdOutputHandle)
	if handle == 0 || handle == ^uintptr(0) {
		return 0, fmt.Errorf("this process has no console output")
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
func Spawn(program string, arguments []string, environment []string) error {
	return spawn(program, arguments, environment, createNewConsole)
}

// SpawnQuietly opens a program with no window at all, for the step in between
// that only works out where the panel belongs.
func SpawnQuietly(program string, arguments []string, environment []string) error {
	return spawn(program, arguments, environment, createNoWindow)
}

// The environment block is UTF-16, and a block that is not said to be one is
// read as ANSI — which Windows refuses rather than mishandles.
func spawn(program string, arguments []string, environment []string, flags uint32) error {
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
		// StringToUTF16 ends each entry with the NUL that ends the string; a
		// second one would end the block right there.
		block = append(block, syscall.StringToUTF16(latest[name])...)
	}
	block = append(block, 0)
	return &block[0]
}

// commandLineFor quotes what needs quoting, which is everything with a space in
// it — a program under `Program Files` first of all.
func commandLineFor(program string, arguments []string) string {
	parts := append([]string{program}, arguments...)
	for index, part := range parts {
		if strings.ContainsAny(part, " \t") {
			parts[index] = `"` + strings.ReplaceAll(part, `"`, `\"`) + `"`
		}
	}
	return strings.Join(parts, " ")
}

// RegisterHotkey claims a key combination for this program, so the panel can be
// opened from wherever the author is working.
func RegisterHotkey(modifiers uint32, key uint32) error {
	if call(procRegisterHotKey, 0, 1, uintptr(modifiers|modNoRepeat), uintptr(key)) == 0 {
		return lastError("RegisterHotKey")
	}
	return nil
}

// WaitForHotkey blocks until the hotkey is pressed and answers false when the
// program is being shut down.
func WaitForHotkey() bool {
	message := message{}
	for {
		result := call(procGetMessageW, uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		switch {
		case result == 0, result == ^uintptr(0):
			return false
		case message.Message == wmHotkey:
			return true
		}
	}
}

// Keys translates a hotkey written the way a person writes one. The letters and
// digits are their own virtual key on Windows, which is all the panel needs.
func Keys(spec string) (uint32, uint32, error) {
	modifiers := uint32(0)
	key := uint32(0)

	for _, part := range strings.Split(strings.ToLower(spec), "+") {
		switch strings.TrimSpace(part) {
		case "ctrl", "control":
			modifiers |= modControl
		case "alt":
			modifiers |= modAlt
		case "shift":
			modifiers |= modShift
		case "win", "super":
			modifiers |= modWin
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
)
