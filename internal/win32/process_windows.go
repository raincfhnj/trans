//go:build windows

package win32

import (
	"syscall"
	"unsafe"
)

// The process tree and the consoles behind it: what Windows is running, and
// which programs share a terminal's input. The decisions made from these live
// in process.go, where they are checked without Windows; this file only asks
// Windows the questions.
var (
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
	procAttachConsole            = kernel32.NewProc("AttachConsole")
	procFreeConsole              = kernel32.NewProc("FreeConsole")
	procGetConsoleProcessList    = kernel32.NewProc("GetConsoleProcessList")
	procGetConsoleTitleW         = kernel32.NewProc("GetConsoleTitleW")
	procGetCurrentProcessId      = kernel32.NewProc("GetCurrentProcessId")
)

// th32csSnapProcess asks the snapshot for every process, which is what the
// parent of each one is read from.
const th32csSnapProcess = 0x2

// processEntry is Windows's PROCESSENTRY32W: the program's name, its id, and
// the id of the process that started it. The fields nothing here reads are
// kept because Windows writes the whole structure into the room it is given.
type processEntry struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

// Processes is everything Windows is running, as a parent-and-child tree. The
// list is a moment rather than a live view: a process that starts after it is
// taken is simply not in it.
func Processes() []Process {
	snapshot, _, _ := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snapshot == 0 || snapshot == invalidHandle {
		return nil
	}
	defer call(procCloseHandle, snapshot)

	entry := processEntry{Size: uint32(unsafe.Sizeof(processEntry{}))}
	kept := []Process{}
	for more, _, _ := procProcess32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&entry))); more != 0; more, _, _ = procProcess32NextW.Call(snapshot, uintptr(unsafe.Pointer(&entry))) {
		kept = append(kept, Process{
			Name:   syscall.UTF16ToString(entry.ExeFile[:]),
			PID:    entry.ProcessID,
			Parent: entry.ParentProcessID,
		})
		entry.Size = uint32(unsafe.Sizeof(processEntry{}))
	}
	return kept
}

// invalidHandle is what CreateToolhelp32Snapshot answers when it fails: the
// handle that is all ones rather than none at all.
const invalidHandle = ^uintptr(0)

// ProcessOfWindow is the program that drew this window: its process, named.
// A window no longer there answers with nothing in it.
func ProcessOfWindow(handle uintptr) Process {
	if handle == 0 {
		return Process{}
	}
	// The call answers with the thread and writes the process beside it, so the
	// room for the process id is what is handed over.
	pid := uint32(0)
	thread := call(procGetWindowThreadProcessID, handle, uintptr(unsafe.Pointer(&pid)))
	if thread == 0 || pid == 0 {
		return Process{}
	}
	for _, process := range Processes() {
		if process.PID == pid {
			return process
		}
	}
	return Process{PID: pid}
}

// SelfPID is this program's own process, for telling a window of its own from
// one belonging to something else.
func SelfPID() uint32 {
	return uint32(call(procGetCurrentProcessId))
}

// SelfWindow says whether this window is the one drawing this program: the
// window of its own console, or the terminal of a process that started it ��
// a panel in a terminal skips the very terminal it is drawing in when it looks
// for the window the author was working in before.
func SelfWindow(handle uintptr, self uint32, processes []Process) bool {
	if handle == 0 {
		return false
	}
	if handle == call(procGetConsoleWindow) {
		return true
	}
	return IsAncestor(processes, ProcessOfWindow(handle).PID, self)
}

// ConsoleMates are the programs sharing this program's own console �� the
// shell that started it and anything that shell is running. A program with no
// console at all says so.
func ConsoleMates() ([]Process, bool) {
	return consoleMates(SelfPID())
}

// consoleMates asks the console the given process is on for its whole list of
// programs. The list is process ids; what they are called is read from the
// tree beside them.
func consoleMates(around uint32) ([]Process, bool) {
	// The list is asked for twice, the way a size is: the first call with room
	// for one id answers how many there are.
	count, _, _ := procGetConsoleProcessList.Call(0, 0)
	if count == 0 {
		return nil, false
	}
	ids := make([]uint32, count+1)
	written, _, _ := procGetConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	if written == 0 {
		return nil, false
	}

	known := map[uint32]Process{}
	for _, process := range Processes() {
		known[process.PID] = process
	}
	mates := make([]Process, 0, written)
	for _, id := range ids[:written] {
		if id == around {
			continue
		}
		process, found := known[id]
		if !found {
			process = Process{PID: id}
		}
		mates = append(mates, process)
	}
	return mates, true
}

// FocusedConsoleMates finds the shell whose console the given window is
// showing and answers what else is on that console. A terminal names its
// window after the pane in front and a shell names its console, so the two
// answering to one another is the window's own pane.
//
// The question is asked by attaching this program to each shell's console in
// turn, which is also how the answer is found: a program with a console of its
// own cannot ask, and says so. What this costs is a few attachments and no
// more; a console that is not the one being looked for is left behind at once.
func FocusedConsoleMates(windowTitle string, processes []Process) ([]Process, bool) {
	if windowTitle == "" || hasConsole() {
		return nil, false
	}
	for _, process := range processes {
		if !IsShell(process.Name) {
			continue
		}
		if !attachConsole(process.PID) {
			continue
		}
		title := consoleTitle()
		mates, listed := consoleMates(SelfPID())
		freeConsole()
		if TitlesMatch(windowTitle, title) {
			return mates, listed
		}
	}
	return nil, false
}

// hasConsole says whether this program already sits on a console of its own.
// Attaching then fails �� Windows allows one console per process �� so the
// callers of FocusedConsoleMates are the ones asking: they run with no console
// of their own at all.
func hasConsole() bool {
	return call(procGetConsoleWindow) != 0
}

func attachConsole(pid uint32) bool {
	attached, _, _ := procAttachConsole.Call(uintptr(pid))
	return attached != 0
}

func freeConsole() {
	call(procFreeConsole)
}

func consoleTitle() string {
	buffer := make([]uint16, 512)
	length, _, _ := procGetConsoleTitleW.Call(
		uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if length == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer[:length])
}
