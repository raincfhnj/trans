//go:build windows

package win32

import (
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procEnumWindows          = user32.NewProc("EnumWindows")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procIsWindow             = user32.NewProc("IsWindow")
	procIsIconic             = user32.NewProc("IsIconic")
	procGetForegroundWindow  = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procGetWindow            = user32.NewProc("GetWindow")
	procMonitorFromRect      = user32.NewProc("MonitorFromRect")
	procGetMonitorInfoW      = user32.NewProc("GetMonitorInfoW")
	procBringWindowToTop     = user32.NewProc("BringWindowToTop")
	procShowWindow           = user32.NewProc("ShowWindow")
	procAttachThreadInput    = user32.NewProc("AttachThreadInput")
	procGetWindowRect        = user32.NewProc("GetWindowRect")
	procGetWindowLongPtr     = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr     = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos         = user32.NewProc("SetWindowPos")
	procGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
	procOpenClipboard        = user32.NewProc("OpenClipboard")
	procEmptyClipboard       = user32.NewProc("EmptyClipboard")
	procCloseClipboard       = user32.NewProc("CloseClipboard")
	procSetClipboardData     = user32.NewProc("SetClipboardData")
	procGetClipboardData     = user32.NewProc("GetClipboardData")
	procRegisterHotKey       = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey     = user32.NewProc("UnregisterHotKey")
	procGetMessageW          = user32.NewProc("GetMessageW")

	procGetConsoleWindow       = kernel32.NewProc("GetConsoleWindow")
	procGetCurrentThreadId     = kernel32.NewProc("GetCurrentThreadId")
	procGetStdHandle           = kernel32.NewProc("GetStdHandle")
	procGlobalAlloc            = kernel32.NewProc("GlobalAlloc")
	procGlobalLock             = kernel32.NewProc("GlobalLock")
	procGlobalUnlock           = kernel32.NewProc("GlobalUnlock")
	procGlobalSize             = kernel32.NewProc("GlobalSize")
	procSetConsoleScreenBuffer = kernel32.NewProc("SetConsoleScreenBufferSize")
	procSetConsoleWindowInfo   = kernel32.NewProc("SetConsoleWindowInfo")
	procSetConsoleTitle        = kernel32.NewProc("SetConsoleTitleW")
	procGetConsoleScreenBuffer = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procCloseHandle            = kernel32.NewProc("CloseHandle")
	procPostThreadMessage      = user32.NewProc("PostThreadMessageW")
	procPostMessageW           = user32.NewProc("PostMessageW")
)

// call runs one Win32 function and returns its result. Anything other than a
// zero result is taken as success, which is how these functions are shaped.
func call(proc *syscall.LazyProc, args ...uintptr) uintptr {
	result, _, _ := proc.Call(args...)
	return result
}

// callWhy runs one Win32 function and returns its result together with the
// error it left behind, for a caller that has to say why it failed.
//
// The error has to come back from the call itself. The last-error word of the
// thread is consumed by the wrapper that makes the call — a GetLastError asked
// for afterwards reads zero however the call went, so a failure reported that
// way once said "was refused" whatever Windows had actually said, including
// "The parameter is incorrect".
func callWhy(proc *syscall.LazyProc, args ...uintptr) (uintptr, error) {
	result, _, why := proc.Call(args...)
	return result, why
}

// lastError is why a call failed, in the words Windows left behind. Not every
// failure sets one, and an error that says nothing is still worth saying.
func lastError(proc string, from error) error {
	if from == nil || errors.Is(from, syscall.Errno(0)) {
		return fmt.Errorf("%s was refused", proc)
	}
	return fmt.Errorf("%s failed: %w", proc, from)
}

type message struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
}

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002

	stdOutputHandle = 0xFFFFFFF5

	swRestore = 9

	wmClose  = 0x0010
	wmHotkey = 0x0312
	// wmReload and wmQuit are the two messages this program posts to itself:
	// one asks the chord loop to read its settings again, the other ends it.
	// They sit in the range Windows leaves to applications.
	wmReload = 0x8000 + 1
	wmQuit   = 0x0012

	modAlt                   = 0x0001
	modControl               = 0x0002
	modShift                 = 0x0004
	modWin                   = 0x0008
	modNoRepeat              = 0x4000
	createNewConsole         = 0x00000010
	createNoWindow           = 0x08000000
	createUnicodeEnvironment = 0x00000400

	vkControl = 0x11
	vkShift   = 0x10
	vkMenu    = 0x12
	vkLWin    = 0x5B
	vkV       = 0x56
	vkReturn  = 0x0D

	gwlStyle   = ^uintptr(15) // -16, the index GetWindowLongPtr wants for the style
	gwlExStyle = ^uintptr(19) // -20, the same for the extended style

	wsCaption    = 0x00C00000
	wsThickFrame = 0x00040000
	wsMinimize   = 0x00020000
	wsMaximize   = 0x00010000
	wsSysMenu    = 0x00080000
	wsVScroll    = 0x00200000
	wsHScroll    = 0x00100000
	wsBorder     = 0x00800000
	wsDlgFrame   = 0x00400000
	wsPopup      = 0x80000000
	wsVisible    = 0x10000000

	wsExWindowEdge = 0x00000100
	wsExClientEdge = 0x00000200
	wsExDlgModal   = 0x00000001
	wsExAppWindow  = 0x00040000

	hwndTopMost = ^uintptr(0)

	// gwHwndNext is the window under this one in the Z-order, which is how a
	// search for the pane a person is working in walks past a helper holding
	// the keyboard.
	gwHwndNext = 2

	// monitorDefaultToNearest is what MonitorFromRect answers with for a place
	// that lies on no monitor at all: the nearest one rather than nothing.
	monitorDefaultToNearest = 2

	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020
	swpShowWindow   = 0x0040
)

// Windows lists the top-level windows a person could point at: the visible ones,
// in the order Windows keeps them.
func Windows() []Window {
	var found []Window

	callback := syscall.NewCallback(func(handle uintptr, _ uintptr) uintptr {
		if opensOver(handle) {
			found = append(found, Window{Handle: handle, Title: Title(handle)})
		}
		return 1
	})

	call(procEnumWindows, callback, 0)
	return found
}

// A window smaller than this is a program's own helper — a message window, a
// tooltip, a tray's hidden window — rather than a pane a person works in. The
// listing of windows, the search for a panel's own window and the choice of a
// pane to open over all draw the line here.
const (
	minPaneWidth  = 200
	minPaneHeight = 120
)

// opensOver says whether a window is one a panel can be opened over: a window
// a person could point at, which is a visible one, large enough to sit on, and
// named. A tray's own hidden window is none of those, and a panel opened over
// one is opened over nothing — while a finished prompt would be pasted into it.
func opensOver(handle uintptr) bool {
	if handle == 0 || call(procIsWindowVisible, handle) == 0 {
		return false
	}
	area := windowRect(handle)
	if area.width() < minPaneWidth || area.height() < minPaneHeight {
		return false
	}
	return Title(handle) != ""
}

// FrontPane is the pane a popup is opened over when nothing named one: the
// window holding the keyboard when it is one a panel can sit on, and the first
// one under it that is when it is not.
//
// A helper window can hold the keyboard. The tray gives its own window the
// foreground before its menu is tracked — Windows insists on it — and the menu
// does not hand it back; the task bar, an input window and a notification can
// take it too. Opening over whatever happens to hold it is how a panel ends up
// centred over a 136-pixel window in the corner of the screen, and how a
// finished prompt would be pasted into a window nobody is looking at.
func FrontPane() Window {
	return FrontPaneExcept(nil)
}

// FrontPaneExcept is FrontPane with the windows left out that are the
// caller's own: a panel drawing in a terminal finds the window it should
// deliver into by skipping the one it is drawing in, and a pane below it in
// the z-order is then the window the author was working in before.
func FrontPaneExcept(skip func(handle uintptr) bool) Window {
	for handle := call(procGetForegroundWindow); handle != 0; handle = call(procGetWindow, handle, gwHwndNext) {
		if !opensOver(handle) {
			continue
		}
		if skip != nil && skip(handle) {
			continue
		}
		return Window{Handle: handle, Title: Title(handle)}
	}
	return Window{}
}

// Foreground is the window the keys are going to right now — the pane a
// keybinding was pressed in when there is no keybinding to ask. It answers with
// whatever holds the keyboard, helper window or not: FrontPane is what finds
// the pane to open over, and a delivery asks this one whether its target still
// holds the keyboard at all.
func Foreground() Window {
	handle := call(procGetForegroundWindow)
	if handle == 0 {
		return Window{}
	}
	return Window{Handle: handle, Title: Title(handle)}
}

func Title(handle uintptr) string {
	length := int32(call(procGetWindowTextLengthW, handle))
	if length <= 0 {
		return ""
	}
	buffer := make([]uint16, length+1)
	if call(procGetWindowTextW, handle, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer))) == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer)
}

// Resolve turns a target setting into the window it names: a handle is checked
// against Windows, a title is looked for among the windows there are.
func Resolve(target Target) (Window, error) {
	if target.Handle != 0 {
		if call(procIsWindow, target.Handle) == 0 {
			return Window{}, fmt.Errorf("the window %#x is gone", target.Handle)
		}
		return Window{Handle: target.Handle, Title: Title(target.Handle)}, nil
	}

	window, found := Find(Windows(), target.Pattern)
	if !found {
		return Window{}, fmt.Errorf("no window matches %q", target.Pattern)
	}
	return window, nil
}

// StillForeground says whether the window is the one holding the keyboard right
// now. Focus is not held for anyone: a click, a notification, or a dialog the
// target opens takes it, and keys sent after that go to whatever took it. The
// moment a key is about to be sent is therefore asked again, not assumed from
// the activation that happened earlier.
func StillForeground(handle uintptr) bool {
	return handle != 0 && Foreground().Handle == handle
}

// Activate brings a window to the front and gives it the keyboard. Windows only
// lets the foreground process change the foreground window, so this borrows the
// thread of whoever holds it for the moment it takes.
func Activate(handle uintptr) error {
	if handle == 0 {
		return errors.New("no window to activate")
	}
	if call(procIsIconic, handle) != 0 {
		call(procShowWindow, handle, swRestore)
	}

	foreground := call(procGetForegroundWindow)
	theirs := uint32(0)
	if foreground != 0 {
		theirs = uint32(call(procGetWindowThreadProcessID, foreground, 0))
	}
	ours := uint32(call(procGetCurrentThreadId))

	attached := false
	if theirs != 0 && theirs != ours {
		attached = call(procAttachThreadInput, uintptr(ours), uintptr(theirs), 1) != 0
	}
	call(procBringWindowToTop, handle)
	call(procSetForegroundWindow, handle)
	if attached {
		call(procAttachThreadInput, uintptr(ours), uintptr(theirs), 0)
	}

	// Windows can refuse, and typing into whatever is in front instead would put
	// the prompt in the wrong place; so this is asked rather than assumed.
	for range 20 {
		if call(procGetForegroundWindow) == handle {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("could not bring %q to the front", Title(handle))
}
