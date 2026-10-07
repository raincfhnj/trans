//go:build windows

package tray

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

// The mark is drawn as a rounded square with the white T-and-arrow inside it
// and clear corners, which is what makes it read as an icon rather than a
// filled block.
func TestTheMarkIsDrawnOnARoundedSquare(t *testing.T) {
	t.Parallel()

	var pixels [iconSize * iconSize * 4]byte
	drawMark(&pixels)

	alpha := func(x, y int) byte { return pixels[(y*iconSize+x)*4+3] }
	white := func(x, y int) bool {
		at := (y*iconSize + x) * 4
		return pixels[at+0] == 255 && pixels[at+1] == 255 && pixels[at+2] == 255
	}

	if alpha(iconSize/2, iconSize/2) == 0 {
		t.Error("the middle of the square is transparent")
	}
	if alpha(0, 0) != 0 {
		t.Error("a corner is drawn solid, want it clear where the square is rounded")
	}

	// The bar and the stem are white, and so is the arrow's tip on the right.
	if !white(13, 13) {
		t.Error("the bar is not drawn white")
	}
	if !white(14, 22) {
		t.Error("the stem is not drawn white")
	}
	if !white(26, 15) {
		t.Error("the arrow is not drawn white")
	}
	// And the square's own colour shows where the mark is not.
	if white(4, 28) {
		t.Error("the background is white where the mark is not")
	}
}

// The icon the shell keeps is this program's to make and to give back, so a
// round trip through drawing and freeing is worth asking for.
func TestTheIconIsMadeAndGivenBack(t *testing.T) {
	t.Parallel()

	handle, err := markIcon()
	if err != nil {
		t.Fatalf("markIcon said %v", err)
	}
	if handle == 0 {
		t.Fatal("markIcon answered with no icon")
	}
	freeIcon(handle)
}

// The tray falls back to the system's own mark by whether there is an error, so
// a failure has to carry one even when Windows leaves no reason behind: one
// that answers nothing is read as a mark of zero with no error at all, and the
// fallback is never taken.
func TestAFailureWithNoReasonIsStillAFailure(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		why  error
	}{
		{"no reason at all", nil},
		{"a reason Windows did not set", windows.Errno(0)},
	} {
		if err := iconFailure("CreateDIBSection", test.why); err == nil {
			t.Errorf("%s: iconFailure answered nothing, want a failure the tray can fall back on", test.name)
		}
	}

	// A reason Windows did leave is kept as it was said.
	given := windows.Errno(5)
	if err := iconFailure("CreateDIBSection", given); !errors.Is(err, given) {
		t.Errorf("iconFailure kept %v out of %v", given, err)
	}
}
