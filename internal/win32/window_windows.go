//go:build windows

package win32

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procEnumWindows            = user32.NewProc("EnumWindows")
	procGetWindowTextW         = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW   = user32.NewProc("GetWindowTextLengthW")
	procIsWindowVisible        = user32.NewProc("IsWindowVisible")
	procIsWindow               = user32.NewProc("IsWindow")
	procIsIconic               = user32.NewProc("IsIconic")
	procGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop       = user32.NewProc("BringWindowToTop")
	procShowWindow             = user32.NewProc("ShowWindow")
	procGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput      = user32.NewProc("AttachThreadInput")
	procGetWindowRect          = user32.NewProc("GetWindowRect")
	procGetWindowLongPtr       = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr       = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos           = user32.NewProc("SetWindowPos")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procOpenClipboard          = user32.NewProc("OpenClipboard")
	procEmptyClipboard         = user32.NewProc("EmptyClipboard")
	procCloseClipboard         = user32.NewProc("CloseClipboard")
	procSetClipboardData       = user32.NewProc("SetClipboardData")
	procGetClipboardData       = user32.NewProc("GetClipboardData")
	procKeybdEvent             = user32.NewProc("keybd_event")
	procRegisterHotKey         = user32.NewProc("RegisterHotKey")
	procGetMessageW            = user32.NewProc("GetMessageW")

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
	procCloseHandle            = kernel32.NewProc("CloseHandle")
)

// call runs one Win32 function and returns its result. Anything other than a
// zero result is taken as success, which is how these functions are shaped.
func call(proc *syscall.LazyProc, args ...uintptr) uintptr {
	result, _, _ := proc.Call(args...)
	return result
}

// lastError is the error the last failed call left behind. Not every failure
// sets one, and an error that says nothing is still worth saying.
func lastError(proc string) error {
	err := syscall.GetLastError()
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("%s was refused", proc)
	}
	return fmt.Errorf("%s failed: %w", proc, err)
}

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }

type coord struct{ X, Y int16 }

type smallRect struct{ Left, Top, Right, Bottom int16 }

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

	wmHotkey = 0x0312

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
	vkV       = 0x56
	vkReturn  = 0x0D
	vkEscape  = 0x1B

	keyEventKeyUp = 0x0002

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
		if call(procIsWindowVisible, handle) == 0 {
			return 1
		}
		// A program keeps small windows of its own under the same title as the
		// one a person works in — a helper, a tooltip — and a panel opened over
		// one of those would be opened over nothing.
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
		if title := syscall.UTF16ToString(buffer); title != "" {
			found = append(found, Window{Handle: handle, Title: title})
		}
		return 1
	})

	call(procEnumWindows, callback, 0)
	return found
}

// Foreground is the window the keys are going to right now — the pane a
// keybinding was pressed in when there is no keybinding to ask.
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

// Activate brings a window to the front and gives it the keyboard. Windows only
// lets the foreground process change the foreground window, so this borrows the
// thread of whoever holds it for the moment it takes.
func Activate(handle uintptr) error {
	if handle == 0 {
		return fmt.Errorf("no window to activate")
	}
	if call(procIsIconic, handle) != 0 {
		call(procShowWindow, handle, swRestore)
	}

	foreground := call(procGetForegroundWindow)
	theirs := uint32(0)
	if foreground != 0 {
		theirs = uint32(call(procGetWindowThreadProcess, foreground, 0))
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
	for attempt := 0; attempt < 20; attempt++ {
		if call(procGetForegroundWindow) == handle {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("could not bring %q to the front", Title(handle))
}
