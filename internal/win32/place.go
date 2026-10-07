package win32

// The shape the placement is written over, and the decision itself that has to
// hold on every screen: where a window is put. It lives here rather than in the
// file that calls Windows because the decision is plain arithmetic — it is
// checked on any system, with no desktop and no call into Windows at all. The
// other Win32 shapes, which nothing but those calls read, are beside it in
// place_windows.go.

type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }

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

// resized is the pixel length a window takes to show want cells where it now
// shows cur of them. It is a ratio rather than a measured cell: how large a
// cell is in pixels belongs to whatever draws the window, and a host that keeps
// a frame round it answers in whole windows. The frame is constant, so the
// answer runs a little large — never so small that the cells asked for are cut
// off — and the caller settles it by measuring again.
//
// A host that shows no cells at all is left as it is rather than divided by
// zero: there is nothing to say what a cell is worth yet.
func resized(now, cur, want int32) int32 {
	if cur <= 0 || want <= 0 {
		return now
	}
	return now * want / cur
}

// capped keeps a length inside a room, never below a floor. A window larger
// than the room cannot be whole in it: the room wins and the caller draws the
// smaller window that fits, rather than one cut off at the screen's edge.
func capped(value, room, floor int32) int32 {
	if room > 0 && value > room {
		value = room
	}
	if value < floor {
		value = floor
	}
	return value
}
