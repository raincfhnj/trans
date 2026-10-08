package overlay_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"trans/internal/overlay"
	"trans/internal/promptflow"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestMain runs this package a few panels at a time rather than GOMAXPROCS of
// them. Every test here drives a Bubble Tea program with a renderer and timers
// of its own, and there are enough of them that starting every one at once
// starves the drawing: a frame that should arrive in a moment takes longer than
// any test is willing to wait, and the test then fails on how loaded the machine
// is rather than on what the panel did. The cap is only applied when nobody
// asked for one — `go test -parallel N` still decides for itself.
func TestMain(m *testing.M) {
	const atOnce = 4
	if asking := flag.Lookup("test.parallel"); asking != nil &&
		asking.Value.String() == strconv.Itoa(runtime.GOMAXPROCS(0)) &&
		runtime.GOMAXPROCS(0) > atOnce {
		_ = flag.Set("test.parallel", strconv.Itoa(atOnce))
	}
	os.Exit(m.Run())
}

const (
	draft   = "Bitte behebe den fehlschlagenden Test"
	english = "Please fix the failing test"
)

type stubTranslator struct {
	english string
	err     error
}

func (s stubTranslator) Translate(context.Context, string) (string, error) {
	return s.english, s.err
}

type recordingTranslator struct {
	english   string
	seenDraft string
}

func (r *recordingTranslator) Translate(_ context.Context, draft string) (string, error) {
	r.seenDraft = draft
	return r.english, nil
}

// recordingTarget stands in for the window a prompt is delivered into. Its
// record is written from the panel's own goroutine and read from the test's,
// so the two meet at the mutex: a test that read the slice while the panel was
// appending to it would be racing the panel, and would fail under -race on the
// day the scheduler happened to interleave them.
type recordingTarget struct {
	mu       sync.Mutex
	inserted []string
}

func (r *recordingTarget) Insert(_ context.Context, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inserted = append(r.inserted, text)
	return nil
}

// sent is what reached the window so far, as a copy: the panel may be
// appending to its own while the test reads this one.
func (r *recordingTarget) sent() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.inserted...)
}

func newOverlay(t *testing.T, translator promptflow.Translator, target promptflow.Target) *teatest.TestModel {
	t.Helper()
	return newOverlayWith(t, translator, target, overlay.Options{
		Service:  "deepl",
		Language: "EN-US",
		Vim:      true,
	})
}

func newOverlayWith(
	t *testing.T,
	translator promptflow.Translator,
	target promptflow.Target,
	options overlay.Options,
	flowOptions ...promptflow.Option,
) *teatest.TestModel {
	t.Helper()
	return teatest.NewTestModel(
		t,
		overlay.New(context.Background(),
			promptflow.New(translator, target, target, flowOptions...), options),
		teatest.WithInitialTermSize(80, 20),
	)
}

// The panel tells a terminal where the caret sits, so that an input method's
// pre-edit text appears where the writing is instead of at the foot of the panel.
func TestTheCaretIsReportedWhereTheWritingIs(t *testing.T) {
	t.Parallel()

	cursor := &overlay.CursorPlace{}
	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{Service: "deepl", Language: "EN-US", Cursor: cursor})

	overlayUnderTest.Type("hallo")

	teatest.WaitFor(t, overlayUnderTest.Output(), func([]byte) bool {
		_, _, _, visible := cursor.Where()
		return visible
	}, teatest.WithDuration(frameTimeout))

	row, column, lines, _ := cursor.Where()
	// The writing begins under the header and the box's top border, one column in
	// for the border and one for the padding — and a terminal counts from one.
	// The caret sits on the last character or just after it, which is where the
	// next one goes.
	if row != 3 {
		t.Errorf("the caret is reported on row %d, want the first row of the draft", row)
	}
	if column < 3+len("hall") || column > 3+len("hallo") {
		t.Errorf("the caret is reported in column %d, want it at the end of the writing", column)
	}
	if lines < 8 {
		t.Errorf("the frame is reported as %d rows, want the rows it was drawn in", lines)
	}

	// A character that takes two cells moves the caret by two, which is what puts
	// an input method's pre-edit text under Chinese writing rather than beside it.
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("你好")})
	teatest.WaitFor(t, overlayUnderTest.Output(), func([]byte) bool {
		_, column, _, _ := cursor.Where()
		return column >= 3+len("hallo")+2 && column <= 3+len("hallo")+4
	}, teatest.WithDuration(frameTimeout))

	closeTheOverlay(t, overlayUnderTest)
}

// A sent prompt leaves the panel open with an empty box, so the next prompt can
// be written without opening it again. These tests wait for that and close the
// panel themselves, the way a person does.
//
// The deadline is an upper bound on how long a drawn frame may take, not a
// promise about how fast the drawing is: the whole suite runs its panels side
// by side, and a machine that is busy — under -race, under -cover, or running
// the other packages at the same time — takes longer than a few seconds to put
// the frame on screen. A test that failed there would be measuring the machine
// rather than the panel.
const frameTimeout = 15 * time.Second

func waitForTheNextPrompt(t *testing.T, overlayUnderTest *teatest.TestModel) {
	t.Helper()
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("ready for the next one"))
	}, teatest.WithDuration(frameTimeout))
}

func closeTheOverlay(t *testing.T, overlayUnderTest *teatest.TestModel) {
	t.Helper()
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestTheOverlayShowsWhichServiceAndLanguageItWillUse(t *testing.T) {
	t.Parallel()

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, &recordingTarget{})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("deepl")) && bytes.Contains(out, []byte("EN-US"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestSubmittingADraftInsertsTheEnglishTranslationIntoTheTarget(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(target.sent()) != 1 || target.sent()[0] != english {
		t.Errorf("target received %v, want one insert of %q", target.sent(), english)
	}

	closeTheOverlay(t, overlayUnderTest)
}

func TestEscapeSwitchesToNormalModeInsteadOfClosing(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Type("hallo")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("NORMAL"))
	}, teatest.WithDuration(frameTimeout))

	if len(target.sent()) != 0 {
		t.Errorf("target received %v, want nothing sent", target.sent())
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

// Escape is the only way back out: once to leave insert mode, once to close.
func TestEscapeTwiceClosesFromNormalMode(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Type("hallo")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	if len(target.sent()) != 0 {
		t.Errorf("target received %v, want nothing sent", target.sent())
	}
}

func TestQIsJustTextWhileTyping(t *testing.T) {
	t.Parallel()

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, &recordingTarget{})
	overlayUnderTest.Type("quatsch")

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("quatsch"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestCancellingLeavesTheTargetUntouched(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	if len(target.sent()) != 0 {
		t.Errorf("target received %v, want nothing inserted", target.sent())
	}
}

func TestAFailedTranslationKeepsTheOverlayOpenAndReportsWhy(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := newOverlay(t, stubTranslator{err: errors.New("deepl unreachable")}, target)
	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("deepl unreachable"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestABlankDraftIsNotSentAnywhere(t *testing.T) {
	t.Parallel()
	translatorThatMustNotRun := stubTranslator{err: errors.New("translator was called")}
	target := &recordingTarget{}

	overlayUnderTest := newOverlay(t, translatorThatMustNotRun, target)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	if len(target.sent()) != 0 {
		t.Errorf("target received %v, want nothing inserted", target.sent())
	}
}

func TestEnterAddsALineInsteadOfSending(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Type("erste Zeile")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEnter})
	overlayUnderTest.Type("zweite Zeile")

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("erste Zeile")) && bytes.Contains(out, []byte("zweite Zeile"))
	}, teatest.WithDuration(frameTimeout))

	if len(target.sent()) != 0 {
		t.Errorf("target received %v, want nothing sent by enter alone", target.sent())
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestAltEnterSendsLikeCtrlD(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(target.sent()) != 1 || target.sent()[0] != english {
		t.Errorf("target received %v, want one insert of %q", target.sent(), english)
	}

	closeTheOverlay(t, overlayUnderTest)
}

func TestADraftKeepsCharactersOutsideAscii(t *testing.T) {
	t.Parallel()
	const umlauts = "Bitte prüfe die Übersetzung: äöü ß — fertig"
	translator := &recordingTranslator{english: english}

	overlayUnderTest := newOverlay(t, translator, &recordingTarget{})
	// Typed byte by byte, teatest would mangle multibyte runes.
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(umlauts)})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if translator.seenDraft != umlauts {
		t.Errorf("translator saw %q, want %q", translator.seenDraft, umlauts)
	}

	closeTheOverlay(t, overlayUnderTest)
}

func TestWithoutVimEscapeClosesTheOverlay(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, target, overlay.Options{
		Service:  "deepl",
		Language: "EN-US",
	})
	overlayUnderTest.Type("hallo")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	if len(target.sent()) != 0 {
		t.Errorf("target received %v, want nothing sent", target.sent())
	}
}

func TestAnEmptyDraftShowsWhatToDo(t *testing.T) {
	t.Parallel()

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, &recordingTarget{})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("own language"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

type gatedTranslator struct {
	mu       sync.Mutex
	calls    int
	started  chan struct{}
	release  chan struct{}
	returned chan struct{}
}

func (g *gatedTranslator) Translate(_ context.Context, _ string) (string, error) {
	g.mu.Lock()
	g.calls++
	call := g.calls
	g.mu.Unlock()

	if call == 1 {
		close(g.started)
		<-g.release
		close(g.returned)
		return "FIRST", nil
	}
	return "SECOND", nil
}

func liveOverlay(t *testing.T, translator promptflow.Translator, target promptflow.Target) *teatest.TestModel {
	t.Helper()
	return newOverlayWith(t, translator, target, overlay.Options{
		Service:  "deepl",
		Language: "EN-US",
		Live:     true,
		Debounce: 20 * time.Millisecond,
	})
}

func TestLiveModeShowsTheEnglishWhileYouWrite(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := liveOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Type(draft)

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(english))
	}, teatest.WithDuration(frameTimeout))

	if len(target.sent()) != 0 {
		t.Errorf("target received %v, want a preview only", target.sent())
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestALatePreviewNeverOverwritesANewerOne(t *testing.T) {
	t.Parallel()
	translator := &gatedTranslator{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		returned: make(chan struct{}),
	}

	overlayUnderTest := liveOverlay(t, translator, &recordingTarget{})
	overlayUnderTest.Type("erste Fassung")
	<-translator.started

	overlayUnderTest.Type(" zweite Fassung")
	// The frame that shows SECOND is the signal the newer preview is up; what
	// comes after the gate opens must not overwrite it.
	var shown []byte
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		if bytes.Contains(out, []byte("SECOND")) {
			shown = append([]byte(nil), out...)
			return true
		}
		return false
	}, teatest.WithDuration(frameTimeout))

	close(translator.release)
	// The late translation is produced now; the overlay either shows it or
	// discards it. Waiting for the translator to have answered is the signal
	// the message is on its way — a sleep would only measure the machine.
	select {
	case <-translator.returned:
	case <-time.After(2 * time.Second):
		t.Fatal("the late translation never returned")
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	final, err := io.ReadAll(overlayUnderTest.Output())
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if bytes.Contains(append(shown, final...), []byte("FIRST")) {
		t.Error("the stale translation was shown, want it discarded")
	}
}

func TestSendingAfterAPreviewDeliversItWithoutTranslatingAgain(t *testing.T) {
	t.Parallel()
	translator := &countingTranslator{english: english}
	target := &recordingTarget{}

	overlayUnderTest := liveOverlay(t, translator, target)
	overlayUnderTest.Type(draft)

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(english))
	}, teatest.WithDuration(frameTimeout))

	// Typing may well have cost more than one translation on the way; what
	// matters is that sending costs none.
	beforeSending := translator.count()

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(target.sent()) != 1 || target.sent()[0] != english {
		t.Errorf("target received %v, want the previewed translation delivered", target.sent())
	}
	if calls := translator.count(); calls != beforeSending {
		t.Errorf("sending cost %d more translations, want the preview reused",
			calls-beforeSending)
	}

	closeTheOverlay(t, overlayUnderTest)
}

type countingTranslator struct {
	mu      sync.Mutex
	calls   int
	english string
}

func (c *countingTranslator) Translate(_ context.Context, _ string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return c.english, nil
}

func (c *countingTranslator) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// A preview belongs to the draft it was made from. Pairing it with the draft
// that merely started the newest request delivers the wrong prompt: the author
// reads one thing and the agent receives another.
func TestSendingWhileANewPreviewIsInFlightNeverDeliversTheOlderEnglish(t *testing.T) {
	t.Parallel()
	translator := &slowSecondTranslator{
		first:   "Do not delete the database",
		second:  "Do not delete the database, delete it after all",
		blocked: make(chan struct{}),
		release: make(chan struct{}),
	}
	target := &recordingTarget{}

	overlayUnderTest := newOverlayWith(t, translator, target, overlay.Options{
		Service:  "deepl",
		Language: "EN-US",
		Live:     true,
		Debounce: 20 * time.Millisecond,
	})

	overlayUnderTest.Type("Loesche die Datenbank nicht")
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Do not delete the database"))
	}, teatest.WithDuration(frameTimeout))

	// The draft changes; the second translation starts but has not answered.
	overlayUnderTest.Type(". Loesche sie doch")
	<-translator.blocked

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	close(translator.release)
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(target.sent()) != 1 {
		t.Fatalf("target received %v, want exactly one delivery", target.sent())
	}
	if delivered := target.sent()[0]; delivered == translator.first {
		t.Errorf("delivered %q, which was translated from the earlier draft", delivered)
	}

	closeTheOverlay(t, overlayUnderTest)
}

type slowSecondTranslator struct {
	mu      sync.Mutex
	calls   int
	first   string
	second  string
	blocked chan struct{}
	release chan struct{}
}

func (s *slowSecondTranslator) Translate(_ context.Context, _ string) (string, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()

	if call == 1 {
		return s.first, nil
	}
	if call == 2 {
		close(s.blocked)
		<-s.release
	}
	return s.second, nil
}

// A draft that moves on makes the request it started pointless; letting it run
// spends characters and request budget on a translation nobody will read.
func TestANewDraftCancelsTheTranslationAlreadyRunning(t *testing.T) {
	t.Parallel()
	translator := &cancellingTranslator{
		entered:  make(chan struct{}),
		observed: make(chan error, 4),
		release:  make(chan struct{}),
	}

	overlayUnderTest := newOverlayWith(t, translator, &recordingTarget{}, overlay.Options{
		Service:  "deepl",
		Language: "EN-US",
		Live:     true,
		Debounce: 20 * time.Millisecond,
	})

	overlayUnderTest.Type("Erste Fassung")
	select {
	case <-translator.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the first translation never started")
	}

	overlayUnderTest.Type(" zweite Fassung")

	select {
	case err := <-translator.observed:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("the abandoned request saw %v, want it cancelled", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("the abandoned request was never cancelled")
	}

	close(translator.release)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestACancelledTranslationIsNotShownAsAFailure(t *testing.T) {
	t.Parallel()
	translator := &cancellingTranslator{
		entered:  make(chan struct{}),
		observed: make(chan error, 4),
		release:  make(chan struct{}),
	}

	overlayUnderTest := newOverlayWith(t, translator, &recordingTarget{}, overlay.Options{
		Service:  "deepl",
		Language: "EN-US",
		Live:     true,
		Debounce: 20 * time.Millisecond,
	})

	overlayUnderTest.Type("Erste Fassung")
	select {
	case <-translator.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the first translation never started")
	}

	overlayUnderTest.Type(" mehr")
	select {
	case <-translator.observed:
	case <-time.After(2 * time.Second):
	}
	close(translator.release)
	// The cancelled request is passed over; the successful one that replaces it
	// is what the frame must show. By the time it does, the failure is either
	// on screen or it was never drawn.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(english)) && !bytes.Contains(out, []byte("context canceled"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	shown, err := io.ReadAll(overlayUnderTest.Output())
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if bytes.Contains(shown, []byte("context canceled")) {
		t.Error("a cancelled request was reported to the author, want it passed over")
	}
}

type cancellingTranslator struct {
	once     sync.Once
	entered  chan struct{}
	observed chan error
	release  chan struct{}
}

func (c *cancellingTranslator) Translate(ctx context.Context, _ string) (string, error) {
	c.once.Do(func() { close(c.entered) })

	select {
	case <-ctx.Done():
		c.observed <- ctx.Err()
		return "", ctx.Err()
	case <-c.release:
		return english, nil
	}
}

func TestEscapeClosesAnEmptyDraftFromNormalMode(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc}) // to normal mode
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc}) // nothing to lose, so close
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	if len(target.sent()) != 0 {
		t.Errorf("target received %v, want nothing sent", target.sent())
	}
}

// Escape closes on written work too, because closing keeps the draft.
func TestEscapeClosesAWrittenDraftAndKeepsIt(t *testing.T) {
	t.Parallel()
	drafts := &fakeDrafts{}

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{Service: "deepl", Language: "EN-US", Vim: true, Drafts: drafts})
	overlayUnderTest.Type("Bitte behebe")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	if len(drafts.saved) == 0 || drafts.saved[len(drafts.saved)-1] != "Bitte behebe" {
		t.Errorf("the store was given %v, want the draft kept on the way out", drafts.saved)
	}
}

type fakeDrafts struct {
	kept      string
	loadError error
	saved     []string
	cleared   int
	saveError error
}

func (f *fakeDrafts) Load() (string, error) { return f.kept, f.loadError }

func (f *fakeDrafts) Save(text string) error {
	if f.saveError != nil {
		return f.saveError
	}
	f.saved = append(f.saved, text)
	return nil
}

func (f *fakeDrafts) Clear() error {
	f.cleared++
	return nil
}

func TestAKeptDraftIsThereAgainWhenTheOverlayOpens(t *testing.T) {
	t.Parallel()
	drafts := &fakeDrafts{kept: "Bitte behebe den Test"}

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{Service: "deepl", Language: "EN-US", Drafts: drafts})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Bitte behebe den Test"))
	}, teatest.WithDuration(frameTimeout))

	// It opens at its beginning, so that is where the cursor is and where writing
	// carries on; the end is a G or an arrow away.
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Gründlich: ")})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Gründlich: Bitte behebe den Test"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestClosingKeepsTheDraftForNextTime(t *testing.T) {
	t.Parallel()
	drafts := &fakeDrafts{}

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{Service: "deepl", Language: "EN-US", Vim: true, Drafts: drafts})
	overlayUnderTest.Type("Bitte behebe den Test")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	if len(drafts.saved) != 1 || drafts.saved[0] != "Bitte behebe den Test" {
		t.Errorf("the store was given %v, want the draft kept once", drafts.saved)
	}
}

func TestASentDraftIsNotKept(t *testing.T) {
	t.Parallel()
	drafts := &fakeDrafts{}
	target := &recordingTarget{}

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, target,
		overlay.Options{Service: "deepl", Language: "EN-US", Drafts: drafts})
	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(target.sent()) != 1 {
		t.Fatalf("target received %v, want the prompt delivered", target.sent())
	}
	if drafts.cleared != 1 {
		t.Errorf("the store was cleared %d times, want the sent draft forgotten", drafts.cleared)
	}
	if len(drafts.saved) != 0 {
		t.Errorf("the store was given %v, want a sent draft not kept", drafts.saved)
	}

	closeTheOverlay(t, overlayUnderTest)
}

func TestADraftThatCannotBeKeptIsNotSilentlyLost(t *testing.T) {
	t.Parallel()
	drafts := &fakeDrafts{saveError: errors.New("disk full")}

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{Service: "deepl", Language: "EN-US", Vim: true, Drafts: drafts})
	overlayUnderTest.Type("Bitte behebe den Test")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("disk full"))
	}, teatest.WithDuration(frameTimeout))

	// ctrl+c is the way out when even keeping the draft fails.
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

// Services charge by the character, and a draft that comes back may be anything —
// yesterday's thought, or a cat on the keyboard. Live translation starts off, so
// resuming costs nothing until ctrl+l asks for it.
func TestAResumedDraftArrivesWithLiveTranslationOff(t *testing.T) {
	t.Parallel()
	translator := &countingTranslator{english: english}
	drafts := &fakeDrafts{kept: "Bitte behebe den Test"}

	overlayUnderTest := newOverlayWith(t, translator, &recordingTarget{},
		overlay.Options{
			Service: "deepl", Language: "EN-US", Live: true,
			Debounce: 10 * time.Millisecond, Drafts: drafts,
		})

	// Live translation turning itself off is worth saying, or it looks broken.
	// That frame is the signal the panel has decided: by the time it says live
	// is off, typing more cannot have asked the service for anything.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Bitte behebe den Test")) &&
			bytes.Contains(out, []byte("translates it"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" gründlich")})

	if calls := translator.count(); calls != 0 {
		t.Errorf("the service was asked %d times for a draft that came back", calls)
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlL})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(english))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

// Closing the popup hangs up on the program, and ctrl+c never reaches the close
// key either. Whatever ends the session, the writing has to survive it.
func TestADraftSurvivesAnEndingNobodyAskedFor(t *testing.T) {
	t.Parallel()
	drafts := &fakeDrafts{}

	target := &recordingTarget{}
	var model tea.Model = overlay.New(context.Background(),
		promptflow.New(stubTranslator{english: english}, target, target),
		overlay.Options{Service: "deepl", Language: "EN-US", Vim: true, Drafts: drafts})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Bitte behebe den Test")})

	if err := model.(overlay.Model).KeepUnfinished(); err != nil {
		t.Fatalf("keeping the draft: %v", err)
	}
	if len(drafts.saved) != 1 || drafts.saved[0] != "Bitte behebe den Test" {
		t.Errorf("the store was given %v, want the draft as it stood", drafts.saved)
	}
}

func TestASentDraftIsNotWrittenBackOnTheWayOut(t *testing.T) {
	t.Parallel()
	drafts := &fakeDrafts{}

	target := &recordingTarget{}
	var model tea.Model = overlay.New(context.Background(),
		promptflow.New(stubTranslator{english: english}, target, target),
		overlay.Options{Service: "deepl", Language: "EN-US", Vim: true, Drafts: drafts})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Bitte behebe den Test")})
	model, _ = model.Update(overlay.PromptDelivered())

	if err := model.(overlay.Model).KeepUnfinished(); err != nil {
		t.Fatalf("keeping the draft: %v", err)
	}
	// The panel stays open after a send, so what is left is an empty box; an
	// empty draft is what the store takes as nothing to keep.
	for _, kept := range drafts.saved {
		if strings.TrimSpace(kept) != "" {
			t.Errorf("the store was given %q, want a sent draft left forgotten", kept)
		}
	}
}

func TestARestoredDraftSaysThatItWasResumed(t *testing.T) {
	t.Parallel()
	drafts := &fakeDrafts{kept: "Bitte behebe den Test"}

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{Service: "deepl", Language: "EN-US", Drafts: drafts})

	// Text that reappears without explanation is a surprise, so say where it
	// came from until the author touches it.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("resumed draft"))
	}, teatest.WithDuration(frameTimeout))

	// A draft that came back opens at its beginning, so that is where writing
	// carries on, and the header stops calling it resumed the moment it is
	// written in. Everything drawn from the first frame that shows the writing
	// is therefore clear of the word: a tail of the output would hold the end
	// of one frame, and the box and the badge are at the top of it.
	overlayUnderTest.Type("!")
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		at := bytes.Index(out, []byte("!Bitte behebe den Test"))
		return at >= 0 && !bytes.Contains(out[at:], []byte("resumed"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestADraftCanBeThrownAwayWithOneKey(t *testing.T) {
	t.Parallel()
	drafts := &fakeDrafts{kept: "Ein alter Entwurf"}

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{Service: "deepl", Language: "EN-US", Drafts: drafts})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Ein alter Entwurf"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlU})
	overlayUnderTest.Type("Etwas Neues")

	// What is written after the throw-away is in the box: the words are typed
	// only after the old draft is gone, so finding them anywhere in what was
	// drawn is finding them after it. The tail of the output is no place to
	// look: a redrawn frame is thousands of bytes and the box sits at the top
	// of it, so the words land outside any fixed tail whenever the renderer
	// paints the frame whole rather than the line that changed.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Etwas Neues"))
	}, teatest.WithDuration(frameTimeout))

	if drafts.cleared != 1 {
		t.Errorf("the store was cleared %d times, want the thrown-away draft forgotten", drafts.cleared)
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func confirmingOverlay(t *testing.T, translator promptflow.Translator, target promptflow.Target) *teatest.TestModel {
	t.Helper()
	return newOverlayWith(t, translator, target, overlay.Options{
		Service:  "deepl",
		Language: "EN-US",
		Confirm:  true,
	})
}

func TestWithConfirmationTheEnglishIsShownBeforeItIsSent(t *testing.T) {
	t.Parallel()
	translator := &countingTranslator{english: english}
	target := &recordingTarget{}

	overlayUnderTest := confirmingOverlay(t, translator, target)
	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(english))
	}, teatest.WithDuration(frameTimeout))

	if len(target.sent()) != 0 {
		t.Fatalf("target received %v, want nothing until it is confirmed", target.sent())
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(target.sent()) != 1 || target.sent()[0] != english {
		t.Errorf("target received %v, want the confirmed translation", target.sent())
	}
	if calls := translator.count(); calls != 1 {
		t.Errorf("the translator was called %d times, want the shown translation reused", calls)
	}

	closeTheOverlay(t, overlayUnderTest)
}

func TestConfirmationCanBeTurnedDownToKeepWriting(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := confirmingOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(english))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.Type(" bitte")

	// Back in the draft, with the writing intact and nothing sent.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Test bitte"))
	}, teatest.WithDuration(frameTimeout))
	if len(target.sent()) != 0 {
		t.Errorf("target received %v, want nothing sent", target.sent())
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestWithoutConfirmationSendingStaysOneKey(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(target.sent()) != 1 {
		t.Errorf("target received %v, want it delivered on the first key", target.sent())
	}

	closeTheOverlay(t, overlayUnderTest)
}

type spendingService struct{ spent promptflow.Usage }

func (s spendingService) Usage(context.Context) (promptflow.Usage, error) {
	return s.spent, nil
}

func TestTheHeaderShowsWhatTheKeyHasSpent(t *testing.T) {
	t.Parallel()
	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{Service: "deepl", Language: "EN-US", Vim: true},
		promptflow.WithUsageReporter(spendingService{
			spent: promptflow.Usage{Used: 12345, Limit: 1_000_000},
		}))

	// Compact, because the header is narrow: 12.3k of 1M.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("12.3k/1M chars"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestAServiceWithoutAnAllowanceShowsNoCount(t *testing.T) {
	t.Parallel()

	overlayUnderTest := newOverlay(t, stubTranslator{english: english}, &recordingTarget{})
	overlayUnderTest.Type("Bitte behebe")
	// The frame that shows the draft is the signal the panel has drawn its
	// header; what that header says about an allowance is read from it.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Bitte behebe"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	shown, err := io.ReadAll(overlayUnderTest.Output())
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if bytes.Contains(shown, []byte("/1M")) || bytes.Contains(shown, []byte("0/0")) {
		t.Error("the header shows an allowance the service never reported")
	}
}

func longOverlay(t *testing.T, translator promptflow.Translator, target promptflow.Target) *teatest.TestModel {
	t.Helper()
	return newOverlayWith(t, translator, target, overlay.Options{
		Service:  "deepl",
		Language: "EN-US",
		Live:     true,
		Debounce: 20 * time.Millisecond,
		MaxDraft: 200,
	})
}

func TestPastingFarMoreThanAPromptSaysSo(t *testing.T) {
	t.Parallel()

	overlayUnderTest := longOverlay(t, stubTranslator{english: english}, &recordingTarget{})
	overlayUnderTest.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(strings.Repeat("sehr langer Text ", 40)),
		Paste: true,
	})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("680 characters"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestTheWarningGoesWhenTheDraftIsShortAgain(t *testing.T) {
	t.Parallel()

	overlayUnderTest := longOverlay(t, stubTranslator{english: english}, &recordingTarget{})
	overlayUnderTest.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(strings.Repeat("x", 400)),
		Paste: true,
	})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("characters"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlU})
	overlayUnderTest.Type("kurz")
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("kurz"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	last, err := io.ReadAll(overlayUnderTest.FinalOutput(t))
	if err != nil {
		t.Fatalf("reading the last frame: %v", err)
	}
	if bytes.Contains(last, []byte("characters")) {
		t.Error("the warning is still shown for a draft that is short again")
	}
}

// Translating a pasted wall of text again after every pause would spend the
// allowance on something the tool is not for.
func TestNothingIsTranslatedWhileTheDraftIsTooLong(t *testing.T) {
	t.Parallel()
	translator := &countingTranslator{english: english}

	overlayUnderTest := longOverlay(t, translator, &recordingTarget{})
	overlayUnderTest.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(strings.Repeat("y", 500)),
		Paste: true,
	})
	// The frame that warns the draft is too long is the signal the panel has
	// decided not to translate it; by the time it says so, nothing was asked.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("characters"))
	}, teatest.WithDuration(frameTimeout))

	if calls := translator.count(); calls != 0 {
		t.Errorf("the translator was called %d times, want the long draft left alone", calls)
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestALongDraftCanStillBeSent(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := longOverlay(t, stubTranslator{english: english}, target)
	overlayUnderTest.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(strings.Repeat("z", 500)),
		Paste: true,
	})
	// The paste warning is the frame the panel draws the moment it has decided;
	// sending before that would race the hint that says live is off.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("pasted"))
	}, teatest.WithDuration(frameTimeout))
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(target.sent()) != 1 {
		t.Errorf("target received %v, want the prompt sent anyway", target.sent())
	}

	closeTheOverlay(t, overlayUnderTest)
}

func TestLiveModeShowsAPulseWhileItIsTranslating(t *testing.T) {
	t.Parallel()
	translator := &gatedTranslator{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		returned: make(chan struct{}),
	}

	overlayUnderTest := newOverlayWith(t, translator, &recordingTarget{}, overlay.Options{
		Service:  "deepl",
		Language: "EN-US",
		Live:     true,
		Pulse:    true,
		Debounce: 20 * time.Millisecond,
	})
	overlayUnderTest.Type("Bitte behebe")
	<-translator.started

	// The circle fills and empties again, so two different states show up.
	seen := map[string]bool{}
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		for _, glyph := range []string{"○", "◔", "◑", "◕", "●"} {
			if bytes.Contains(out, []byte(glyph)) {
				seen[glyph] = true
			}
		}
		return len(seen) >= 2
	}, teatest.WithDuration(frameTimeout))

	close(translator.release)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func TestWithoutThePulseLiveModeSaysSoQuietly(t *testing.T) {
	t.Parallel()
	translator := &gatedTranslator{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		returned: make(chan struct{}),
	}

	overlayUnderTest := newOverlayWith(t, translator, &recordingTarget{}, overlay.Options{
		Service:  "deepl",
		Language: "EN-US",
		Live:     true,
		Debounce: 20 * time.Millisecond,
	})
	overlayUnderTest.Type("Bitte behebe")
	<-translator.started
	// The frame that mentions live is the signal the panel has drawn its
	// header; the pulse would have filled the circle by now if it were on.
	var shown []byte
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		if bytes.Contains(out, []byte("live")) {
			shown = append([]byte(nil), out...)
			return true
		}
		return false
	}, teatest.WithDuration(frameTimeout))

	close(translator.release)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	if !bytes.Contains(shown, []byte("live")) {
		t.Error("live mode is not mentioned at all")
	}
	for _, glyph := range []string{"◔", "◑", "◕"} {
		if bytes.Contains(shown, []byte(glyph)) {
			t.Errorf("the header pulses with %q although the pulse is off", glyph)
		}
	}
}

// Nothing is being translated, so the circle rests instead of drawing attention.
func TestThePulseRestsWhenNothingIsBeingTranslated(t *testing.T) {
	t.Parallel()

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{Service: "deepl", Language: "EN-US", Live: true, Pulse: true})
	// The header is the frame the panel draws on its own; by the time it names
	// the service, nothing has been asked of it, so the circle rests.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("deepl")) && bytes.Contains(out, []byte("live"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))

	shown, err := io.ReadAll(overlayUnderTest.Output())
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	for _, filling := range []string{"◔", "◑", "◕", "●"} {
		if bytes.Contains(shown, []byte(filling)) {
			t.Errorf("the circle filled to %q with nothing to translate", filling)
		}
	}
}

// A translation that answers in a blink would otherwise flash once and stop, so
// the circle finishes the breath it started.
func TestAFastTranslationStillShowsAWholeBreath(t *testing.T) {
	t.Parallel()

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{
			Service:  "deepl",
			Language: "EN-US",
			Live:     true,
			Pulse:    true,
			Debounce: 20 * time.Millisecond,
		})
	overlayUnderTest.Type("Bitte behebe")

	seen := map[string]bool{}
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		for _, glyph := range []string{"·", "○", "◔", "◑", "◕", "●"} {
			if bytes.Contains(out, []byte(glyph)) {
				seen[glyph] = true
			}
		}
		return len(seen) >= 5
	}, teatest.WithDuration(4*time.Second))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

func switchableOverlay(t *testing.T, sending, typing promptflow.Target, options overlay.Options) *teatest.TestModel {
	t.Helper()
	flow := promptflow.New(stubTranslator{english: english}, sending, typing)
	options.Service, options.Language = "deepl", "EN-US"
	return teatest.NewTestModel(t, overlay.New(context.Background(), flow, options),
		teatest.WithInitialTermSize(87, 17))
}

// Whether a prompt is sent or only typed is easier to decide once the draft is
// written, so the key is in the popup rather than in the keybinding.
func TestCtrlRSwitchesToTypingWithoutSending(t *testing.T) {
	t.Parallel()
	sending, typing := &recordingTarget{}, &recordingTarget{}

	overlayUnderTest := switchableOverlay(t, sending, typing, overlay.Options{})
	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlR})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("fills the input"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(typing.sent()) != 1 || typing.sent()[0] != english {
		t.Errorf("the typing target received %v, want the prompt", typing.sent())
	}
	if len(sending.sent()) != 0 {
		t.Errorf("the sending target received %v, want nothing", sending.sent())
	}

	closeTheOverlay(t, overlayUnderTest)
}

func TestCtrlRSwitchesBackToSending(t *testing.T) {
	t.Parallel()
	sending, typing := &recordingTarget{}, &recordingTarget{}

	overlayUnderTest := switchableOverlay(t, sending, typing, overlay.Options{Review: true})
	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlR})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(sending.sent()) != 1 {
		t.Errorf("the sending target received %v, want the prompt", sending.sent())
	}
	if len(typing.sent()) != 0 {
		t.Errorf("the typing target received %v, want nothing", typing.sent())
	}

	closeTheOverlay(t, overlayUnderTest)
}

// Live translation costs characters, so it can be turned on for one prompt.
func TestCtrlLTurnsLiveTranslationOnForThisPrompt(t *testing.T) {
	t.Parallel()
	translator := &countingTranslator{english: english}
	flow := promptflow.New(translator, &recordingTarget{}, &recordingTarget{})

	overlayUnderTest := teatest.NewTestModel(t,
		overlay.New(context.Background(), flow, overlay.Options{
			Service: "deepl", Language: "EN-US", Debounce: 20 * time.Millisecond,
		}),
		teatest.WithInitialTermSize(87, 17))

	overlayUnderTest.Type("Bitte behebe")
	// The frame that shows the draft is the signal the panel has drawn its
	// header with live off; by the time it does, nothing was asked of the
	// service, and ctrl+l is what asks.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Bitte behebe"))
	}, teatest.WithDuration(frameTimeout))
	if calls := translator.count(); calls != 0 {
		t.Fatalf("the translator ran %d times with live off", calls)
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlL})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(english))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}
