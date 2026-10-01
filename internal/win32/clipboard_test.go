//go:build windows

package win32

import "testing"

// Which formats hold a handle rather than a block of memory: locking one of
// these reads a handle as an address, which is how a clipboard holding a
// drawing takes the process down. The bitmap is not asked about here — it is
// copied with CopyImage by name before this question comes up — and the ranges
// matter as much as the named formats, because a program can register anything
// on the clipboard.
func TestAFormatThatIsAHandleIsNotReadAsMemory(t *testing.T) {
	t.Parallel()

	handles := []uint32{
		cfPalette,
		cfMetafilePict,
		cfEnhMetafile,
		cfDSPMetafilePict,
		cfDSPEnhMetafile,
		cfDSPBitmap,
		cfOwnerDisplay,
		cfDSPText,
		cfPrivate,
		cfGDIObjectFirst,
		cfGDIObjectFirst + 7,
		cfGDIObjectLast,
	}
	for _, id := range handles {
		if !handleShaped(id) {
			t.Errorf("format %d is read as memory, want it left alone as a handle", id)
		}
	}

	memory := []uint32{
		cfUnicodeText,
		cfText,
		cfOemText,
		cfHDrop,
		cfLocale,
		cfDIB,
		cfDIBV5,
		cfGDIObjectFirst - 1,
		cfGDIObjectLast + 1,
	}
	for _, id := range memory {
		if handleShaped(id) {
			t.Errorf("format %d is left alone as a handle, want it copied as memory", id)
		}
	}
}
