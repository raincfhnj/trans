// Package tray gives the daemon an icon in the notification area with the few
// things a daemon has to offer: open the panel, open the settings, read the
// settings again, start at logon, and quit.
//
// It runs a message loop of its own. The daemon's chords belong to the thread
// that claimed them — RegisterHotKey is thread-affine — so the tray cannot
// take them over; it has a hidden window and a loop on a thread of its own and
// only ever asks the daemon's thread to do something, by posting to it. The
// two loops meet nowhere else.
package tray

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Item is one line of the tray's menu.
type Item int

const (
	OpenPanel Item = iota + 1
	ClosePanel
	Settings
	Reload
	StartAtLogon
	Quit
)

// The words the menu shows. They are here rather than in the daemon because
// they belong to the menu, not to what the items do.
var labels = map[Item]string{
	OpenPanel:    "Open panel",
	ClosePanel:   "Close panel",
	Settings:     "Settings\u2026",
	Reload:       "Reload settings",
	StartAtLogon: "Start at logon",
	Quit:         "Quit",
}

// Options is what the daemon hands the tray: what to call for each item, and
// how to draw the two menu marks that are state rather than action.
type Options struct {
	// Tip is the text that appears when the pointer rests on the icon.
	Tip string
	// Do runs one menu item. It is called on the tray's own thread, so
	// anything the daemon's thread owns has to be asked for by posting to it.
	Do func(Item)
	// LogonChecked says whether the start-at-logon entry carries its tick.
	LogonChecked func() bool
	// LogonAvailable says whether this installation can be started at logon at
	// all: a daemon running out of a temporary directory has no stable path to
	// put in the registry, and an entry pointing at a file that will be gone
	// is worse than no entry.
	LogonAvailable func() bool
}

// notifyIconData is NOTIFYICONDATAW, the structure the shell reads to know
// what to draw. The order and the widths are the API's, not a choice.
type notifyIconData struct {
	Size             uint32
	Window           windows.HWND
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             windows.Handle
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	Guid             windows.GUID
	BalloonIcon      windows.Handle
}

type point struct{ X, Y int32 }

type msg struct {
	Window  windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
	Private uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  windows.Handle
}

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	user32  = windows.NewLazySystemDLL("user32.dll")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procAppendMenuW      = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procSetForegroundWin = user32.NewProc("SetForegroundWindow")
	procLoadIconW        = user32.NewProc("LoadIconW")
	procGetModuleHandleW = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

const (
	windowClassName = "trans-tray"

	// The shell answers on this message when the icon is clicked or the menu
	// asked for; it is the first message Windows leaves to applications.
	callbackMessage = 0x8000 + 1

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	wmDestroy   = 0x0002
	wmCommand   = 0x0111
	wmRButtonUp = 0x0205
	wmLButtonUp = 0x0202
	wmNull      = 0x0000

	mfString    = 0x00000000
	mfChecked   = 0x00000008
	mfGrayed    = 0x00000001
	mfSeparator = 0x00000800

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	idiApplication = 32512
)

// ErrUnavailable is what a system without a notification area answers.
var ErrUnavailable = errors.New("tray: no notification area on this system")

// Run puts the icon up and pumps its messages until Stop is called, Quit is
// chosen, or the program ends. It takes the thread it is on for the lifetime
// of the loop, because a window belongs to the thread that created it, so it
// is run from a goroutine of its own.
func Run(options Options) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	state := &trayState{options: options}
	if err := state.create(); err != nil {
		return err
	}
	defer state.remove()

	finished := make(chan struct{})
	mu.Lock()
	active, quit = state, finished
	mu.Unlock()
	defer func() {
		mu.Lock()
		active, quit = nil, nil
		mu.Unlock()
		close(finished)
	}()

	// A right click opens the menu; anything else is left to the shell. The
	// message is the WndProc's own argument, so it is declared here and lives
	// as long as the call does.
	message := msg{}
	for {
		result, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		switch result {
		case 0, ^uintptr(0):
			return nil
		}
		call(procTranslateMessage, uintptr(unsafe.Pointer(&message)))
		call(procDispatchMessageW, uintptr(unsafe.Pointer(&message)))
	}
}

var (
	mu sync.Mutex
	// active is the icon currently up, and quit is closed when its loop ends.
	// The daemon asks for both through Stop, which is the only other thread
	// that touches them.
	active *trayState
	quit   chan struct{}
)

// Stop takes the icon away and waits for its loop to end, so the program can
// exit without leaving an icon behind that a click would open nothing with.
// Calling it when no tray is running does nothing.
func Stop() {
	mu.Lock()
	state, finished := active, quit
	mu.Unlock()
	if state == nil || state.window == 0 {
		return
	}
	call(procPostMessageW, uintptr(state.window), wmDestroy, 0, 0)
	if finished == nil {
		return
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		// A loop that will not end is not worth hanging the exit on; the
		// process is going away and Windows takes the icon with it.
	}
}

// call runs one Win32 function and drops what it answers. These are the calls
// whose failure has nothing the program can do about it: a menu item the shell
// would not draw, a window that will not come forward. The ones whose answer
// matters are called with Call directly and checked.
func call(proc *windows.LazyProc, args ...uintptr) uintptr {
	result, _, _ := proc.Call(args...)
	return result
}

type trayState struct {
	options Options
	window  windows.HWND
	icon    notifyIconData
	menu    windows.Handle
}

func (state *trayState) create() error {
	instance, _, _ := procGetModuleHandleW.Call(0)

	className, err := windows.UTF16PtrFromString(windowClassName)
	if err != nil {
		return err
	}
	class := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   syscall.NewCallback(state.windowProc),
		Instance:  windows.Handle(instance),
		ClassName: className,
	}
	if result, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return fmt.Errorf("tray: registering the window class: %w", err)
	}

	window, _, callErr := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)),
		0, 0, 0, 0, 0, 0, 0, instance, 0)
	if window == 0 {
		return fmt.Errorf("tray: creating the window: %w", callErr)
	}
	state.window = windows.HWND(window)

	// IDI_APPLICATION is the icon the system already has. A tray icon of our
	// own would mean shipping a resource file for a program that never draws
	// anything else.
	icon, _, _ := procLoadIconW.Call(0, uintptr(idiApplication))

	state.icon = notifyIconData{
		Window:          state.window,
		ID:              1,
		Flags:           nifMessage | nifIcon | nifTip,
		CallbackMessage: callbackMessage,
		Icon:            windows.Handle(icon),
	}
	copy(state.icon.Tip[:], windows.StringToUTF16(state.options.Tip))
	state.icon.Size = uint32(unsafe.Sizeof(state.icon))
	if err := state.shell(nimAdd); err != nil {
		return err
	}
	return nil
}

func (state *trayState) shell(action uint32) error {
	if result, _, err := procShellNotifyIconW.Call(uintptr(action), uintptr(unsafe.Pointer(&state.icon))); result == 0 {
		if action == nimDelete {
			// Taking the icon away twice is not worth an error.
			return nil
		}
		return fmt.Errorf("tray: Shell_NotifyIcon: %w", err)
	}
	return nil
}

func (state *trayState) remove() {
	_ = state.shell(nimDelete)
	if state.menu != 0 {
		call(procDestroyMenu, uintptr(state.menu))
	}
	if state.window != 0 {
		call(procDestroyWindow, uintptr(state.window))
	}
}

// windowProc is where the shell's callbacks arrive. Everything but the menu
// is left to the default handler.
func (state *trayState) windowProc(window uintptr, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case callbackMessage:
		if lparam == wmRButtonUp || lparam == wmLButtonUp {
			state.showMenu()
		}
		return 0
	case wmCommand:
		state.chose(Item(loword(uint32(wparam))))
		return 0
	case wmDestroy:
		call(procPostQuitMessage, 0)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(window, uintptr(message), wparam, lparam)
	return result
}

func loword(value uint32) uint32 { return value & 0xFFFF }

// showMenu builds the menu each time it is opened, so the tick beside
// start-at-logon and the words of the items are read from the daemon as they
// are now rather than as they were when the daemon started.
func (state *trayState) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer call(procDestroyMenu, menu)

	for _, item := range []Item{OpenPanel, ClosePanel, Settings, Reload, StartAtLogon, Quit} {
		if item == StartAtLogon && !state.logonAvailable() {
			continue
		}
		flags := uintptr(mfString)
		if item == StartAtLogon {
			if state.options.LogonChecked != nil && state.options.LogonChecked() {
				flags |= mfChecked
			}
		}
		label, err := windows.UTF16PtrFromString(labels[item])
		if err != nil {
			continue
		}
		call(procAppendMenuW, menu, flags, uintptr(item), uintptr(unsafe.Pointer(label)))
	}

	position := point{}
	call(procGetCursorPos, uintptr(unsafe.Pointer(&position)))
	// The classic sequence: the window has to be in front before the menu is
	// tracked, or the menu stays on screen after the click that opened it.
	call(procSetForegroundWin, uintptr(state.window))
	chosen, _, _ := procTrackPopupMenu.Call(menu,
		tpmRightButton|tpmReturnCmd, uintptr(position.X), uintptr(position.Y),
		0, uintptr(state.window), 0)
	call(procPostMessageW, uintptr(state.window), wmNull, 0, 0)

	if chosen != 0 {
		state.chose(Item(chosen))
	}
}

func (state *trayState) logonAvailable() bool {
	if state.options.LogonAvailable == nil {
		return false
	}
	return state.options.LogonAvailable()
}

func (state *trayState) chose(item Item) {
	if state.options.Do == nil {
		return
	}
	state.options.Do(item)
	if item == Quit && state.window != 0 {
		call(procPostMessageW, uintptr(state.window), wmDestroy, 0, 0)
	}
}
