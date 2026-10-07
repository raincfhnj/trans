//go:build windows

package tray

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The mark the tray shows is the panel's own: a bar with a stem that reads as a
// T, and an arrow saying what the panel does with a draft, on a rounded square.
// It is drawn here rather than shipped as a resource so the program stays one
// file to hand round and the mark is this program's own.
const (
	// iconSize is the width and height of the icon bitmap the shell is handed.
	// It is drawn well above the size the notification area shows, so the
	// scaling it does keeps the edges of the mark smooth rather than blocky.
	iconSize = 64
	// shapeSize is the side of the space the mark itself is drawn in, so the
	// shapes below can be written once in whole numbers and the bitmap can be
	// made as large as the shell likes without rewriting them.
	shapeSize = 32.0
	// iconCorners is how far the square's corners are rounded, in the shape's
	// own space.
	iconCorners = 8.0
	// iconSample is how many points each pixel is judged at, per side. A pixel
	// on the edge of the square or the mark is then part of both, which is what
	// turns a staircase of blocks into a smooth edge.
	iconSample = 4
)

var (
	gdiIcon = windows.NewLazySystemDLL("gdi32.dll")

	procCreateDIBSection   = gdiIcon.NewProc("CreateDIBSection")
	procDeleteObject       = gdiIcon.NewProc("DeleteObject")
	procCreateBitmap       = gdiIcon.NewProc("CreateBitmap")
	procCreateIconIndirect = user32.NewProc("CreateIconIndirect")
	procDestroyIcon        = user32.NewProc("DestroyIcon")
)

// bitmapInfoHeader is BITMAPINFOHEADER, the shape CreateDIBSection reads. The
// widths are the API's, not a choice.
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// iconInfo is ICONINFO, what CreateIconIndirect is handed.
type iconInfo struct {
	Icon     int32
	HotspotX uint32
	HotspotY uint32
	Mask     windows.Handle
	Color    windows.Handle
}

// markIcon draws the panel's mark as a notification-area icon. The colour comes
// back as an icon the shell keeps until it is destroyed with freeIcon, so the
// caller owns both and the two are handed on together.
func markIcon() (windows.Handle, error) {
	// A top-down 32bpp DIB: the pixels are handed back to be written directly,
	// which is the only way to draw the square's rounded corners with the alpha
	// that makes them transparent.
	header := bitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:    iconSize,
		Height:   -iconSize,
		Planes:   1,
		BitCount: 32,
	}
	var bits unsafe.Pointer
	color, _, why := procCreateDIBSection.Call(
		0, uintptr(unsafe.Pointer(&header)), 0,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == nil {
		return 0, iconFailure("CreateDIBSection", why)
	}
	colorBitmap := windows.Handle(color)

	pixels := (*[iconSize * iconSize * 4]byte)(bits)
	drawMark(pixels)

	// The mask says which pixels the screen shows through. Per-pixel alpha
	// already keeps the corners clear, but a mask that agrees with it is what
	// older hosts read.
	mask := make([]byte, iconSize*iconSize/8)
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			if !insideSquare(shapeAt(x), shapeAt(y)) {
				mask[y*iconSize/8+x/8] |= 0x80 >> uint(x%8)
			}
		}
	}
	maskBitmap, _, why := procCreateBitmap.Call(
		iconSize, iconSize, 1, 1, uintptr(unsafe.Pointer(&mask[0])))
	if maskBitmap == 0 {
		call(procDeleteObject, uintptr(colorBitmap))
		return 0, iconFailure("CreateBitmap", why)
	}

	information := iconInfo{Icon: 1, Mask: windows.Handle(maskBitmap), Color: colorBitmap}
	handle, _, why := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&information)))
	call(procDeleteObject, maskBitmap)
	call(procDeleteObject, uintptr(colorBitmap))
	if handle == 0 {
		return 0, iconFailure("CreateIconIndirect", why)
	}
	return windows.Handle(handle), nil
}

// freeIcon gives the icon back once the shell is done with it.
func freeIcon(handle windows.Handle) {
	if handle != 0 {
		call(procDestroyIcon, uintptr(handle))
	}
}

// iconFailure is why a call that draws the mark failed, and it never answers
// with nothing. The reason has to come back from the call itself: the wrapper
// that makes a call reads the thread's last-error word on its way out, so a
// GetLastError asked for afterwards answers with nothing however the call went
// — which is how a mark that could not be drawn was reported as an icon of zero
// with no error at all, and the tray drew nothing instead of falling back to
// the system's own mark.
func iconFailure(proc string, why error) error {
	if why == nil || errors.Is(why, windows.Errno(0)) {
		return fmt.Errorf("%s was refused", proc)
	}
	return fmt.Errorf("%s failed: %w", proc, why)
}

// shapeAt maps a pixel coordinate on the bitmap to the shape's own space.
func shapeAt(pixel int) float64 {
	return (float64(pixel) + 0.5) * shapeSize / iconSize
}

// drawMark fills the pixels with the rounded square and the mark on it. Each
// pixel is judged at a grid of points rather than at its middle alone, so a
// pixel on an edge is a little of both sides and the mark reads as smooth
// rather than as a staircase of blocks.
func drawMark(pixels *[iconSize * iconSize * 4]byte) {
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			// Colour and coverage summed over the samples within the pixel.
			var red, green, blue, cover float64
			for sy := 0; sy < iconSample; sy++ {
				for sx := 0; sx < iconSample; sx++ {
					px := (float64(x) + (float64(sx)+0.5)/iconSample) * shapeSize / iconSize
					py := (float64(y) + (float64(sy)+0.5)/iconSample) * shapeSize / iconSize
					if !insideSquare(px, py) {
						continue
					}
					r, g, b := squareColour(px, py)
					if insideMark(px, py) {
						r, g, b = 255, 255, 255
					}
					red += float64(r)
					green += float64(g)
					blue += float64(b)
					cover++
				}
			}

			at := (y*iconSize + x) * 4
			// The DIB's pixels are blue, green, red, alpha, in that order. The
			// alpha is how much of the pixel the square covers, which is what
			// keeps the rounded corners clear.
			if cover == 0 {
				pixels[at+0], pixels[at+1], pixels[at+2], pixels[at+3] = 0, 0, 0, 0
				continue
			}
			total := float64(iconSample * iconSample)
			pixels[at+0] = byte(blue / cover)
			pixels[at+1] = byte(green / cover)
			pixels[at+2] = byte(red / cover)
			pixels[at+3] = byte(255 * cover / total)
		}
	}
}

// insideSquare is whether a point is within the rounded square.
func insideSquare(x, y float64) bool {
	const lo = 2.0
	hi := shapeSize - 2.0
	r := iconCorners
	if x < lo || x > hi || y < lo || y > hi {
		return false
	}
	// The corners are quarter circles; the middle of each edge is straight.
	near := func(value, edge float64) float64 {
		if value > edge {
			return value - edge
		}
		if value < edge {
			return edge - value
		}
		return 0
	}
	cx := near(x, lo+r)
	if x > hi-r {
		cx = x - (hi - r)
	}
	cy := near(y, lo+r)
	if y > hi-r {
		cy = y - (hi - r)
	}
	if x > lo+r && x < hi-r {
		cx = 0
	}
	if y > lo+r && y < hi-r {
		cy = 0
	}
	return cx*cx+cy*cy <= r*r
}

// squareColour is the square's background at a point: a diagonal gradient from
// an indigo at the top left to a blue at the bottom right, as the icon has.
func squareColour(x, y float64) (red, green, blue int) {
	t := (x + y) / (2 * shapeSize)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return int(lerp(93, 37, t)), int(lerp(49, 99, t)), int(lerp(201, 235, t))
}

func lerp(from, to, t float64) float64 { return from + (to-from)*t }

// insideMark is whether a pixel is part of the white T-and-arrow.
func insideMark(x, y float64) bool {
	// The bar across the top.
	if x >= 7 && x <= 20 && y >= 11 && y <= 16 {
		return true
	}
	// The stem down from its middle.
	if x >= 12 && x <= 17 && y >= 15 && y <= 25 {
		return true
	}
	// The arrow pointing out of the bar's right end.
	return insideTriangle(x, y, 19, 8, 19, 21, 28, 14.5)
}

// insideTriangle is whether a point is within a triangle, by which side of each
// edge it is on. All three agree on the inside, whichever way the points run.
func insideTriangle(x, y, x1, y1, x2, y2, x3, y3 float64) bool {
	sign := func(ax, ay, bx, by, cx, cy float64) float64 {
		return (ax-cx)*(by-cy) - (bx-cx)*(ay-cy)
	}
	d1 := sign(x, y, x1, y1, x2, y2)
	d2 := sign(x, y, x2, y2, x3, y3)
	d3 := sign(x, y, x3, y3, x1, y1)
	hasNegative := d1 < 0 || d2 < 0 || d3 < 0
	hasPositive := d1 > 0 || d2 > 0 || d3 > 0
	return !hasNegative || !hasPositive
}
