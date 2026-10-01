//go:build windows

package win32

import (
	"errors"
	"fmt"
	"sort"
	"unsafe"
)

// SnapshotClipboard copies what is on the clipboard so that it can be put back
// after a prompt has borrowed it for the moment the pane needs to take it.
// Everything that can be copied is copied; a clipboard whose only content is a
// format that cannot be is reported rather than overwritten, because losing a
// screenshot or a set of files beats worse than not delivering a prompt.
//
// The snapshot holds memory of its own and keeps it until Restore has handed it
// over or Release has freed it, so a caller takes one of the two once it is
// done.
func SnapshotClipboard() (ClipboardSnapshot, error) {
	if !openClipboard() {
		return nil, errors.New("the clipboard is held by something else")
	}
	defer closeClipboard()

	snapshot := &clipboardSnapshot{}

	// Only one of these is ever on the clipboard: Windows keeps them in step
	// with each other, and the plain bitmap is the one older programs write.
	bitmap := call(procGetClipboardData, cfBitmap)
	if bitmap == 0 {
		bitmap = call(procGetClipboardData, cfDSPBitmap)
	}
	if bitmap != 0 {
		// A picture that cannot be copied as a bitmap may still come back as
		// CF_DIB below, which is memory and is copied like everything else, so
		// this failure is not answered yet.
		if copied, err := copyBitmap(bitmap); err == nil {
			snapshot.bitmap = copied
		}
	}

	// The length of the list is asked for one format at a time rather than
	// counted first: another program can add a format while this walks it, and
	// EnumClipboardFormats answers with the next one each time.
	for id := uint32(0); ; {
		id = uint32(call(procEnumClipboardFormats, uintptr(id)))
		if id == 0 {
			break
		}
		if _, handleShaped := unrepeatableFormats[id]; !handleShaped {
			if memory, err := copyFormat(id); err == nil {
				snapshot.formats = append(snapshot.formats, clipboardFormat{id: id, memory: memory})
			}
			continue
		}
		if id != cfBitmap {
			continue
		}
		// A bitmap handle is not memory, so it is put back as CF_DIB, which is.
		// A clipboard that has the bitmap and no device-independent copy of it
		// is the one case where the picture cannot be taken away safely, and it
		// is refused rather than emptied.
		if snapshot.bitmap == 0 && !snapshot.hasFormat(cfDIB) && !snapshot.hasFormat(cfDIBV5) {
			snapshot.Release()
			note("a clipboard holding a bitmap that cannot be copied was left alone")
			return nil, errors.New("the clipboard holds a picture that cannot be copied, so it was not overwritten")
		}
	}

	// The standard formats come first in the order above, so a program that
	// reads text reads a text format even though the clipboard held others too.
	sort.SliceStable(snapshot.formats, func(first, second int) bool {
		return copiedFormats[snapshot.formats[first].id] < copiedFormats[snapshot.formats[second].id]
	})
	return snapshot, nil
}

// clipboardSnapshot is the snapshot itself: the global memory blocks copied out
// of the clipboard, and the picture, which is a handle rather than memory and
// is copied with CopyImage.
type clipboardSnapshot struct {
	formats  []clipboardFormat
	bitmap   uintptr
	restored bool
}

// Restore puts the clipboard back as the snapshot found it, and takes the
// copied memory out of this struct's hands: what SetClipboardData accepts
// belongs to the clipboard from then on and is never freed here.
//
// A restore that cannot put everything back does not throw away what it has
// already put back — a clipboard holding the text but not the picture is better
// than an empty one — and the failed call's memory is freed with the snapshot.
func (snapshot *clipboardSnapshot) Restore() error {
	if snapshot == nil || snapshot.restored {
		return nil
	}
	if !openClipboard() {
		return errors.New("the clipboard is held by something else, so what was on it could not be put back")
	}
	defer closeClipboard()

	call(procEmptyClipboard)

	// Windows drops a format written twice in a row, and CF_DIB is written here
	// both for the bitmap that was copied and as one of the copied formats.
	put := map[uint32]bool{}

	if snapshot.bitmap != 0 {
		if call(procSetClipboardData, cfBitmap, snapshot.bitmap) == 0 {
			return fmt.Errorf("putting the picture back on the clipboard: %w", lastError("SetClipboardData"))
		}
		snapshot.bitmap = 0
		put[cfBitmap] = true
		put[cfDSPBitmap] = true
	}

	for index := range snapshot.formats {
		format := &snapshot.formats[index]
		if put[format.id] {
			continue
		}
		if call(procSetClipboardData, uintptr(format.id), format.memory) == 0 {
			return fmt.Errorf("putting format %d back on the clipboard: %w", format.id, lastError("SetClipboardData"))
		}
		put[format.id] = true
		format.memory = 0
	}

	snapshot.restored = true
	return nil
}

// Release frees whatever this snapshot still holds: everything when it was
// never put back, and nothing but the picture when it was. It never touches the
// clipboard itself.
func (snapshot *clipboardSnapshot) Release() {
	if snapshot == nil {
		return
	}
	for index := range snapshot.formats {
		format := &snapshot.formats[index]
		if format.memory != 0 {
			call(procGlobalFree, format.memory)
			format.memory = 0
		}
	}
	snapshot.formats = nil
	if snapshot.bitmap != 0 {
		// The copy belongs to this program rather than to the clipboard, and is
		// deleted the way any other bitmap is.
		call(procDeleteObject, snapshot.bitmap)
		snapshot.bitmap = 0
	}
}

// Holds says whether there is anything to put back, which is what decides
// whether opening the clipboard again is worth it.
func (snapshot *clipboardSnapshot) Holds() bool {
	return snapshot != nil && (snapshot.bitmap != 0 || len(snapshot.formats) > 0)
}

// clipboardFormat is one format copied out of the clipboard: the format id and
// the global memory holding a private copy of the clipboard's own block.
type clipboardFormat struct {
	id     uint32
	memory uintptr
}

// copiedFormats is where the formats this program knows by name go back in the
// order they are put back: the text formats first, because a program that reads
// text reads the first one it understands, then the file list, the locale that
// says how the text is encoded, and the device-independent picture.
//
// The list is short on purpose. Windows has hundreds of registered formats — an
// application writes its own alongside the standard ones — and a format this
// program has never heard of is still copied when it is memory, which is what
// EnumClipboardFormats is there for. The order in this list is the only thing
// the list decides.
var copiedFormats = map[uint32]int{
	cfUnicodeText: 0,
	cfText:        1,
	cfOemText:     2,
	cfHDrop:       3,
	cfLocale:      4,
	cfDIB:         5,
	cfDIBV5:       6,
}

// unrepeatableFormats are the formats the clipboard stores as a handle to
// something rather than as a block of memory. CopyImage can copy a bitmap and
// nothing else, so the rest are refused: a palette, a metafile or a private
// display format cannot be read out of the clipboard without becoming something
// else, and a clipboard holding only those is better left untouched than
// emptied.
//
// A bitmap is not in this map because it is copied with CopyImage; when that
// fails it still comes back as CF_DIB or CF_DIBV5, which are memory.
var unrepeatableFormats = map[uint32]string{
	cfPalette:      "a colour palette",
	cfMetafilePict: "a metafile picture",
	cfEnhMetafile:  "an enhanced metafile",
	cfDSPBitmap:    "a private bitmap",
	cfOwnerDisplay: "a drawing only the window that put it there can render",
	cfDSPText:      "a private text format",
	cfGDIObj:       "a graphics object",
	cfPrivate:      "a private format",
}

// hasFormat says whether a format was copied out of the clipboard. A format the
// clipboard stores as a handle is not in the list at all, so this only ever
// answers about memory.
func (snapshot *clipboardSnapshot) hasFormat(id uint32) bool {
	for _, format := range snapshot.formats {
		if format.id == id {
			return true
		}
	}
	return false
}

// copyFormat makes a private copy of the clipboard's memory for one format. The
// clipboard's own handle is borrowed and released again: GlobalLock counts its
// uses, and the block itself still belongs to the clipboard until it is closed.
func copyFormat(id uint32) (uintptr, error) {
	handle := call(procGetClipboardData, uintptr(id))
	if handle == 0 {
		return 0, errors.New("the clipboard no longer holds that format")
	}
	pointer := call(procGlobalLock, handle)
	if pointer == 0 {
		return 0, lastError("GlobalLock")
	}
	defer call(procGlobalUnlock, handle)

	size := int(call(procGlobalSize, handle))
	if size <= 0 {
		return 0, errors.New("the clipboard's memory has no size")
	}
	memory := call(procGlobalAlloc, gmemMoveable, uintptr(size))
	if memory == 0 {
		return 0, lastError("GlobalAlloc")
	}
	copied := call(procGlobalLock, memory)
	if copied == 0 {
		call(procGlobalFree, memory)
		return 0, lastError("GlobalLock")
	}
	copy(cells(copied, size), cells(pointer, size))
	call(procGlobalUnlock, memory)
	return memory, nil
}

// copyBitmap copies a picture off the clipboard with CopyImage, which is the
// one call that copies a handle-shaped format at all. LR_COPYRETURNORG hands
// the original back when it can be handed back — which is exact, and is what a
// bitmap made by CreateDIBSection needs — and a converted copy is taken when it
// cannot. The copy belongs to this program, not to the clipboard, so it is
// deleted with DeleteObject unless a restore hands it over.
func copyBitmap(handle uintptr) (uintptr, error) {
	copied := call(procCopyImage, handle, imageBitmap, 0, 0, lrCopyReturnOrg)
	if copied == 0 {
		copied = call(procCopyImage, handle, imageBitmap, 0, 0, lrCreateDIB)
	}
	if copied == 0 {
		return 0, lastError("CopyImage")
	}
	return copied, nil
}

// cells is memory GlobalLock handed back, seen as the bytes it holds. The
// address is Windows's, not the collector's, and what it points at is the
// clipboard's own memory: it is valid from the lock above until
// CloseClipboard, and it is never kept past the call that returned it.
//
// This is the one place in the package where a number from Win32 becomes a
// pointer, and `go vet` reports conversions like it as a possible misuse of
// unsafe.Pointer wherever they are written. That is a known limit of vet with
// Win32 FFI and not something this code can be written around; the invariant
// above is what makes it safe.
func cells(address uintptr, size int) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(address)), size) //nolint:gosec,govet // the address belongs to the clipboard
}

// The clipboard formats this package copies by hand, and the flags its calls
// are made with. The numbers are Windows's own; the formats are the ones with a
// meaning beyond "a block of memory", which is why they are named here.
const (
	cfBitmap       = 2
	cfMetafilePict = 3
	cfDIB          = 8
	cfPalette      = 9
	cfEnhMetafile  = 14
	cfHDrop        = 15
	cfLocale       = 16
	cfDIBV5        = 17
	cfText         = 1
	cfOemText      = 7

	// The private formats: the ones an application keeps to itself, and
	// CF_OWNERDISPLAY, which is not data at all but a window that draws itself
	// wherever the clipboard is shown.
	cfOwnerDisplay = 0x0080
	cfDSPText      = 0x0081
	cfDSPBitmap    = 0x0082
	cfGDIObj       = 0x0083
	cfPrivate      = 0x0200

	imageBitmap = 0

	// LR_COPYRETURNORG returns the original handle when it can be returned
	// unchanged, which is exact; LR_CREATEDIBSECTION asks for a copy that is
	// usable as a device-independent bitmap, which is what the clipboard wants.
	lrCopyReturnOrg = 0x00000008
	lrCreateDIB     = 0x00002000
)
