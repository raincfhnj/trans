package win32

// The shapes the Win32 calls answer with, and the one decision written over
// them that has to hold on every screen: where a window is put. The shapes are
// here rather than in the files that call Windows because that decision is
// plain arithmetic — it is checked on any system, with no desktop and no call
// into Windows at all.

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }

type coord struct{ X, Y int16 }

type smallRect struct{ Left, Top, Right, Bottom int16 }

// placedWithin is where the top left of a window of this size goes: centred
// over the place it belongs on, and kept inside the room the screen gives it.
//
// Centring alone is what strands a panel outside the screen. A window larger
// than the pane it opens over — every panel is larger than a helper window a
// program keeps of its own — is centred past that pane's edges, and the
// arithmetic then answers with negative coordinates: the window is moved off
// the screen with whatever it has up there, and there is nothing left to drag
// or click. Keeping the window inside the room is what leaves the position one
// a person can reach, whatever the pane turned out to be.
//
// A window larger than the room is pinned to the room's near corner: it cannot
// be whole, and its top left is where a person starts reading it. The room is
// the work area of a monitor, so a pane on a screen addressed with negative
// coordinates keeps them.
func placedWithin(width, height int32, over, room rect) (left, top int32) {
	left = over.Left + (over.width()-width)/2
	top = over.Top + (over.height()-height)/2
	return pinned(left, room.Left, room.Right-width),
		pinned(top, room.Top, room.Bottom-height)
}

// pinned keeps a value between two edges. When the edges are the wrong way
// round — a window wider than the room it is placed in — the near edge wins.
func pinned(value, near, far int32) int32 {
	switch {
	case far < near, value < near:
		return near
	case value > far:
		return far
	default:
		return value
	}
}
