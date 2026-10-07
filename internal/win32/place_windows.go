//go:build windows

package win32

// The Win32 shapes that only the Windows half answers with, kept beside the one
// in place.go. rect is there because the placement arithmetic written over it is
// checked on any system; these three are read by nothing but the calls
// themselves, and a type that nothing outside the Windows files use is a type
// the build for another system has no place for — which is exactly what the
// lint on that build says.
//
// The widths are Windows's: POINT is two LONG, COORD is two SHORT, and
// SMALL_RECT is four of them.
type point struct{ X, Y int32 }

type coord struct{ X, Y int16 }

type smallRect struct{ Left, Top, Right, Bottom int16 }
