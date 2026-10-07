package win32

import "testing"

// The window a popup draws is larger than many of the panes it is opened over,
// and centring it there is what used to leave it outside the screen: the
// arithmetic answers with negative coordinates and the window is moved to them,
// title bar and all. Every case here is a place a real popup ended up in.
func TestAWindowIsPutOverThePaneItBelongsOn(t *testing.T) {
	t.Parallel()

	left, top := placedWithin(200, 100,
		rect{Left: 100, Top: 50, Right: 500, Bottom: 400},
		rect{Left: 0, Top: 0, Right: 1920, Bottom: 1080})

	if left != 200 || top != 175 {
		t.Errorf("placedWithin centred the window at %d,%d, want 200,175", left, top)
	}
}

// A pane in the corner of a screen leaves no room to centre in: the window is
// pulled back over the screen's edge rather than placed past it.
func TestAWindowOverAPaneAtTheScreenEdgeStaysOnTheScreen(t *testing.T) {
	t.Parallel()

	left, top := placedWithin(400, 200,
		rect{Left: 1800, Top: 1000, Right: 1920, Bottom: 1080},
		rect{Left: 0, Top: 0, Right: 1920, Bottom: 1080})

	if left != 1520 || top != 880 {
		t.Errorf("placedWithin put the window at %d,%d, want 1520,880: the room's far edge", left, top)
	}
}

// A tray's own hidden window is 136x39 in the corner of the screen, and a
// panel centred over it used to be moved to -469,-171 — outside the screen,
// undraggable and unclickable, with only the tray left to close it. The window
// now lands in the room it is given instead.
func TestAPanelOverAHelperWindowInTheCornerStaysOnTheScreen(t *testing.T) {
	t.Parallel()

	left, top := placedWithin(1075, 381,
		rect{Left: 0, Top: 0, Right: 136, Bottom: 39},
		rect{Left: 0, Top: 0, Right: 1707, Bottom: 1067})

	if left < 0 || top < 0 {
		t.Errorf("placedWithin stranded the panel at %d,%d, want it inside the screen", left, top)
	}
	if left != 0 || top != 0 {
		t.Errorf("placedWithin put the panel at %d,%d, want 0,0: the near corner of the room", left, top)
	}
}

// A window larger than the screen it is on cannot be whole anywhere. Its top
// left is where a person starts reading it, so that is what is kept.
func TestAWindowLargerThanTheRoomIsPinnedToItsNearCorner(t *testing.T) {
	t.Parallel()

	left, top := placedWithin(2000, 1200,
		rect{Left: 400, Top: 300, Right: 900, Bottom: 700},
		rect{Left: 0, Top: 0, Right: 1707, Bottom: 1067})

	if left != 0 || top != 0 {
		t.Errorf("placedWithin put the window at %d,%d, want 0,0", left, top)
	}
}

// A screen to the left of the primary one is addressed with negative
// coordinates, and a window belongs there with them: clamping to zero would
// move the panel to a screen the pane is not on.
func TestAWindowOnAScreenAddressedWithNegativeCoordinatesKeepsThem(t *testing.T) {
	t.Parallel()

	left, top := placedWithin(400, 300,
		rect{Left: -1800, Top: 0, Right: -1000, Bottom: 800},
		rect{Left: -1920, Top: 0, Right: 0, Bottom: 1080})

	if left != -1600 || top != 250 {
		t.Errorf("placedWithin put the window at %d,%d, want -1600,250", left, top)
	}
}

func TestPinnedKeepsAValueBetweenItsEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		value, near, far int32
		want             int32
	}{
		{name: "a value between the edges is left alone", value: 5, near: 0, far: 10, want: 5},
		{name: "a value past the near edge is pulled back", value: -3, near: 0, far: 10, want: 0},
		{name: "a value past the far edge is pulled back", value: 12, near: 0, far: 10, want: 10},
		{name: "edges the wrong way round answer the near one", value: 5, near: 10, far: 0, want: 10},
		{name: "a negative room keeps its coordinates", value: -700, near: -800, far: -100, want: -700},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := pinned(test.value, test.near, test.far); got != test.want {
				t.Errorf("pinned(%d, %d, %d) = %d, want %d",
					test.value, test.near, test.far, got, test.want)
			}
		})
	}
}

// A window is made large enough for the cells the overlay draws, whatever the
// host's cell turns out to be in pixels. The answer is a ratio, so a host that
// draws big cells asks for a big window and one that draws small ones does not.
func TestResizedScalesTheWindowToTheCellsWanted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		now, cur, want, result int32
	}{
		{"a window already showing what is asked is left alone", 800, 100, 100, 800},
		{"a window showing half the cells doubles", 400, 50, 100, 800},
		{"a window showing twice the cells halves", 800, 200, 100, 400},
		{"a host that shows no cells leaves the window as it is", 800, 0, 100, 800},
		{"a want of nothing leaves the window as it is", 800, 100, 0, 800},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := resized(test.now, test.cur, test.want); got != test.result {
				t.Errorf("resized(%d, %d, %d) = %d, want %d",
					test.now, test.cur, test.want, got, test.result)
			}
		})
	}
}

// The screen is the only thing that makes a panel smaller than the content it
// draws, and it never makes one so small there is nothing to see.
func TestCappedKeepsASizeInsideTheRoom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                       string
		value, room, floor, result int32
	}{
		{"a size inside the room is left alone", 500, 800, 0, 500},
		{"a size past the room is pulled back to it", 900, 800, 0, 800},
		{"a size under the floor is lifted to it", 100, 800, 200, 200},
		{"the floor beats a room too small for it", 100, 50, 200, 200},
		{"no room is no cap", 900, 0, 0, 900},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := capped(test.value, test.room, test.floor); got != test.result {
				t.Errorf("capped(%d, %d, %d) = %d, want %d",
					test.value, test.room, test.floor, got, test.result)
			}
		})
	}
}
