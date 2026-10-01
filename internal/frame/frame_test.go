package frame_test

import (
	"strings"
	"testing"

	"trans/internal/frame"

	"github.com/charmbracelet/lipgloss"
)

func TestSpreadPinsBothEndsToTheLine(t *testing.T) {
	t.Parallel()

	line := frame.Spread(" esc close", "ctrl+c also closes", 40)
	if lipgloss.Width(line) != 40 {
		t.Errorf("spread drew %d cells, want the line exactly: %q", lipgloss.Width(line), line)
	}
	if !strings.HasPrefix(line, " esc close") {
		t.Errorf("the left end is %q, want the beginning of the line", line)
	}
	if !strings.HasSuffix(line, "ctrl+c also closes") {
		t.Errorf("the right end is %q, want the end of the line", line)
	}
}

// Too narrow for both ends, the keys at the start are what a person is looking
// for; a wrapped footer would push the box up and the popup out of the pane.
func TestSpreadKeepsTheBeginningWhenBothEndsDoNotFit(t *testing.T) {
	t.Parallel()

	line := frame.Spread(" esc close", "ctrl+c also closes", 10)
	if lipgloss.Width(line) > 10 {
		t.Errorf("spread drew %d cells for a line of 10: %q", lipgloss.Width(line), line)
	}
	if !strings.HasPrefix(line, " esc clos") {
		t.Errorf("the narrow line is %q, want the keys it can hold", line)
	}
}

func TestTheLabelIsWrittenIntoTheBottomBorder(t *testing.T) {
	t.Parallel()
	styles := frame.NewStyles()

	box := frame.Box(true, 40).Render("the writing")
	labelled := frame.Labelled(&styles.Mark, &styles.Badge, &styles.Badge, box, "45%", true)

	border := strings.Split(labelled, "\n")
	if !strings.Contains(border[len(border)-1], "45%") {
		t.Errorf("the label is not in the bottom border:\n%s", labelled)
	}
	if !strings.Contains(labelled, "the writing") {
		t.Errorf("the box lost its contents:\n%s", labelled)
	}
}

func TestALabelThatDoesNotFitIsLeftOut(t *testing.T) {
	t.Parallel()
	styles := frame.NewStyles()

	box := frame.Box(true, 14).Render("text")
	if labelled := frame.Labelled(&styles.Mark, &styles.Badge, &styles.Badge, box, "a-label-far-too-long", true); labelled != box {
		t.Errorf("the box is %q, want it untouched by a label without room", labelled)
	}
}

func TestTheBarTellsWhereTheViewSits(t *testing.T) {
	t.Parallel()
	styles := frame.NewStyles()
	body := strings.Repeat("a line of text\n", 20)

	shown := frame.Scrolled(&styles, 30, strings.TrimSuffix(body, "\n"), 0, 5, 20)

	if !strings.Contains(shown, frame.ScrollThumb) {
		t.Error("the view sits in text that does not fit, want a thumb to show where")
	}
	if !strings.Contains(shown, frame.ScrollTrack) {
		t.Error("only the thumb was drawn, want the track under it")
	}
	for _, row := range strings.Split(shown, "\n") {
		if want := 30 + 1 + lipgloss.Width(frame.ScrollThumb); lipgloss.Width(row) != want {
			t.Errorf("the row is %d cells, want the text padded to %d with the bar as its column: %q",
				lipgloss.Width(row), want, row)
			break
		}
	}
}

func TestTextThatFitsHasNoBar(t *testing.T) {
	t.Parallel()
	styles := frame.NewStyles()

	body := "one line\nand another"
	shown := frame.Scrolled(&styles, 30, body, 0, 5, 2)
	if shown != body {
		t.Errorf("Scrolled drew %q, want the text left as it stands", shown)
	}
}

func TestRowsOfCountsWrappedRows(t *testing.T) {
	t.Parallel()

	if rows := frame.RowsOf("", 20); rows != 1 {
		t.Errorf("RowsOf(empty) = %d, want the one row an empty box still has", rows)
	}
	if rows := frame.RowsOf(strings.Repeat("word ", 30), 20); rows < 3 {
		t.Errorf("RowsOf of a long text = %d, want it counted across the rows it wraps to", rows)
	}
	if rows := frame.RowsOf("exactly fits this line", 22); rows != 1 {
		t.Errorf("RowsOf of one fitting line = %d, want 1", rows)
	}
}
