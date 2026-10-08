package frame_test

import (
	"strconv"
	"testing"

	"trans/internal/frame"
)

// The theme row steps through ThemeNames and starts on the one with no name;
// anything else — an empty TRANS_THEME, a name nobody has heard of — draws
// with the default rather than the panel refusing to open over a colour it
// cannot find.
func TestAnUnknownThemeDrawsWithTheDefaultPalette(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "auto", "default", "nord-ish"} {
		if got := frame.PaletteFor(name); got != frame.DefaultPalette {
			t.Errorf("PaletteFor(%q) = %+v, want the default palette %+v", name, got, frame.DefaultPalette)
		}
	}
}

// The names are what the settings window steps through, in order: the default
// palette is the row's starting position ("auto"), not one of them.
func TestTheThemeNamesAreTheBuiltInsInSteppingOrder(t *testing.T) {
	t.Parallel()

	want := []string{"ocean", "forest", "amber", "mono"}
	got := frame.ThemeNames()
	if len(got) != len(want) {
		t.Fatalf("ThemeNames() = %q, want %q", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("ThemeNames()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

// A theme only points at the terminal's own palette — a slot outside 0-15
// would be a colour the terminal's theme never chose, and grey is never the
// accent or the danger, or what is meant to be read would be drawn in the
// colour of the furniture. This is the rule the popups' own tests rely on.
func TestEveryThemePointsAtTheTerminalsPaletteAndMeaningfully(t *testing.T) {
	t.Parallel()

	palettes := map[string]frame.Palette{"auto": frame.DefaultPalette}
	for _, name := range frame.ThemeNames() {
		palettes[name] = frame.PaletteFor(name)
	}
	for name, palette := range palettes {
		for slot, colour := range map[string]string{
			"accent": string(palette.Accent),
			"danger": string(palette.Danger),
			"grey":   string(palette.Grey),
			"bright": string(palette.Bright),
		} {
			number, err := strconv.Atoi(colour)
			if err != nil || number < 0 || number > 15 {
				t.Errorf("%s theme: %s is slot %q, want one of the terminal's own 0-15", name, slot, colour)
			}
		}
		if palette.Accent == palette.Grey || palette.Danger == palette.Grey {
			t.Errorf("%s theme paints what is read in the furniture's grey: %+v", name, palette)
		}
	}
}
