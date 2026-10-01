package translation_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"trans/internal/translation"
)

const codeBlock = "```go\nif kunde == nil { // sollte nie passieren\n\treturn fmt.Errorf(\"kunde fehlt\")\n}\n```"

// A service asked to translate code renames the identifiers in it, so the code is
// never sent: it is taken out, and put back exactly as it was.
func TestACodeBlockIsNeitherSentNorChanged(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	draft := "Der Fehler steht hier:\n\n" + codeBlock + "\n\nBitte behebe das."
	translated, err := translator.Translate(context.Background(), draft)
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	if !strings.Contains(translated, codeBlock) {
		t.Errorf("the code came back as %q, want it untouched", translated)
	}
	for _, sent := range service.sent() {
		if strings.Contains(sent, "kunde") || strings.Contains(sent, "fmt.Errorf") {
			t.Errorf("the code was sent to the service: %q", sent)
		}
	}
}

func TestAnInlineCodeSpanIsNeitherSentNorChanged(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	translated, err := translator.Translate(context.Background(),
		"Schreibe einen Test für `berechneRabatt(warenkorb, prozent)` bitte.")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	if !strings.Contains(translated, "`berechneRabatt(warenkorb, prozent)`") {
		t.Errorf("the function name came back as %q, want it untouched", translated)
	}
	if sent := strings.Join(service.sent(), " "); strings.Contains(sent, "berechneRabatt") {
		t.Errorf("the function name was sent to the service: %q", sent)
	}
}

// A span may be broken across a line break: both backticks still close it, so
// what sits between them is code and stays out of the request.
func TestAnInlineSpanAcrossALineBreakIsNeitherSentNorChanged(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	draft := "Der Fehler liegt in `berechneRabatt\n(warenkorb)` und nirgendwo sonst."
	translated, err := translator.Translate(context.Background(), draft)
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	if !strings.Contains(translated, "`berechneRabatt\n(warenkorb)`") {
		t.Errorf("the span came back as %q, want it untouched", translated)
	}
	sent := strings.Join(service.sent(), " ")
	if strings.Contains(sent, "berechneRabatt") {
		t.Errorf("the span was sent to the service: %q", sent)
	}
	if !strings.Contains(sent, "Der Fehler liegt") {
		t.Errorf("the service was sent %q, want the prose around the span", sent)
	}
}

// The prose still goes as whole sentences, with a marker standing in for the code,
// or the service would be translating fragments.
func TestTheProseAroundTheCodeIsStillTranslatedAsASentence(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	if _, err := translator.Translate(context.Background(),
		"Ersetze `alterName` durch `neuerName` im Formular."); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	sent := strings.Join(service.sent(), " ")
	if !strings.Contains(sent, "Ersetze") || !strings.Contains(sent, "im Formular") {
		t.Errorf("the service was sent %q, want the sentence around the code", sent)
	}
	if strings.Count(sent, "⟦") != 2 {
		t.Errorf("the service was sent %q, want a marker for each protected span", sent)
	}
}

func TestADraftWithoutCodeIsPassedStraightThrough(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	translated, err := translator.Translate(context.Background(), "Bitte behebe den Test.")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if translated != "<Bitte behebe den Test.>" {
		t.Errorf("Translate returned %q, want the plain answer", translated)
	}
	if sent := strings.Join(service.sent(), " "); strings.Contains(sent, "⟦") {
		t.Errorf("the service was sent a marker for a draft with no code: %q", sent)
	}
}

// A block being typed has no closing fence yet, and half-written code is the least
// worth translating.
func TestAnUnfinishedCodeBlockIsProtectedToTheEnd(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	draft := "Hier der Fehler:\n\n```go\nif kunde == nil {"
	translated, err := translator.Translate(context.Background(), draft)
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	if !strings.Contains(translated, "```go\nif kunde == nil {") {
		t.Errorf("the unfinished block came back as %q, want it untouched", translated)
	}
	if sent := strings.Join(service.sent(), " "); strings.Contains(sent, "kunde") {
		t.Errorf("the unfinished block was sent: %q", sent)
	}
}

// A lone backtick opens a span still being typed. Until its closing one appears,
// everything after it is held back — the same fails-closed rule an unclosed fence
// follows — so a live preview between the two keystrokes never sends the span.
func TestAnUnfinishedInlineSpanIsProtectedToTheEnd(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	translated, err := translator.Translate(context.Background(),
		"Das Zeichen ` steht auf der Tastatur oben links.")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	if !strings.Contains(translated, "` steht auf der Tastatur oben links.") {
		t.Errorf("the unfinished span came back as %q, want it untouched", translated)
	}
	sent := strings.Join(service.sent(), " ")
	if strings.Contains(sent, "Tastatur") {
		t.Errorf("the unfinished span was sent to the service: %q", sent)
	}
	if !strings.Contains(sent, "Das Zeichen") {
		t.Errorf("the service was sent %q, want the prose before the backtick", sent)
	}
}

// A draft can hold a fence and inline spans at once: each is taken out on its
// own, and one protected span must not disturb the next.
func TestFencesAndInlineSpansAreProtectedInTheSameDraft(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	draft := "Erst `init()` rufen, dann:\n\n" + codeBlock + "\n\nDanach `close()` nicht vergessen."
	translated, err := translator.Translate(context.Background(), draft)
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	for _, kept := range []string{"`init()`", codeBlock, "`close()`"} {
		if !strings.Contains(translated, kept) {
			t.Errorf("the protected %q did not come back in %q", kept, translated)
		}
	}
	sent := strings.Join(service.sent(), " ")
	if strings.Contains(sent, "init()") || strings.Contains(sent, "close()") || strings.Contains(sent, "kunde") {
		t.Errorf("code was sent to the service: %q", sent)
	}
	if !strings.Contains(sent, "Erst") || !strings.Contains(sent, "Danach") {
		t.Errorf("the service was sent %q, want the prose around the code", sent)
	}
	if strings.Count(sent, "⟦") != 3 {
		t.Errorf("the service was sent %q, want a marker for each of the three spans", sent)
	}
}

// There is nothing to protect in an empty or blank draft, and no marker may
// appear where the draft went through untouched.
func TestAnEmptyOrBlankDraftGoesThroughWithoutAMarker(t *testing.T) {
	t.Parallel()

	for _, draft := range []string{"", "   ", "\n\n\t "} {
		service := &spyTranslator{}
		translated, err := translation.Protecting(service).Translate(context.Background(), draft)
		if err != nil {
			t.Fatalf("Translate(%q) returned unexpected error: %v", draft, err)
		}
		if sent := service.sent(); len(sent) != 1 || sent[0] != draft {
			t.Errorf("Translate(%q) sent %q, want the draft itself", draft, sent)
		}
		if strings.Contains(translated, "⟦") {
			t.Errorf("Translate(%q) returned %q, want no marker for a blank draft", draft, translated)
		}
	}
}

// An echo answers with exactly what it was given, so a draft must come back
// byte for byte: whatever takeOut removes, putBack restores.
type echoTranslator struct {
	mu  sync.Mutex
	got []string
}

func (e *echoTranslator) Translate(_ context.Context, text string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.got = append(e.got, text)
	return text, nil
}

func TestTakingOutAndPuttingBackIsTheIdentityOnTheDraft(t *testing.T) {
	t.Parallel()

	for _, draft := range []string{
		"",
		"   \n\t",
		"Bitte behebe den Test.",
		"Schreibe `berechneRabatt(warenkorb, prozent)` bitte.",
		"Der Fehler liegt in `berechneRabatt\n(warenkorb)` und nirgendwo sonst.",
		"Das Zeichen ` steht auf der Tastatur oben links.",
		"Hier der Fehler:\n\n```go\nif kunde == nil {",
		"Erst `init()` rufen, dann:\n\n" + codeBlock + "\n\nDanach `close()`.",
		"Die Notation ⟦0⟧ kommt aus der Mathematik.",
	} {
		echo := &echoTranslator{}
		translated, err := translation.Protecting(echo).
			Translate(context.Background(), draft)
		if err != nil {
			t.Errorf("Translate(%q) returned unexpected error: %v", draft, err)
			continue
		}
		if translated != draft {
			t.Errorf("Translate(%q) returned %q, want the draft exactly as it was", draft, translated)
		}
		// The identity is only worth anything if something really was taken out.
		if len(echo.got) == 1 && echo.got[0] == draft && strings.Contains(draft, "`") {
			t.Errorf("Translate(%q) was sent verbatim, want it marked over the code", draft)
		}
	}
}

type markerLosingTranslator struct{ spyTranslator }

func (m *markerLosingTranslator) Translate(ctx context.Context, text string) (string, error) {
	translated, err := m.spyTranslator.Translate(ctx, text)
	if err != nil {
		return "", err
	}
	// A service that drops a marker takes the code with it.
	if at := strings.Index(translated, "⟦"); at >= 0 {
		if end := strings.Index(translated[at:], "⟧"); end >= 0 {
			translated = translated[:at] + translated[at+end+len("⟧"):]
		}
	}
	return translated, nil
}

// Delivering a prompt with the code silently missing is worse than saying so.
func TestAMarkerTheServiceLostIsAnErrorRatherThanAQuietLoss(t *testing.T) {
	t.Parallel()
	translator := translation.Protecting(&markerLosingTranslator{})

	_, err := translator.Translate(context.Background(),
		"Der Fehler steht hier:\n\n"+codeBlock+"\n\nBitte behebe das.")
	if err == nil {
		t.Fatal("Translate returned no error for a translation that lost the code")
	}
	if !strings.Contains(err.Error(), "protected") {
		t.Errorf("Translate returned %v, want it to say a protected part went missing", err)
	}
}

// Someone writing the marker characters themselves must not collide with ours.
func TestTextThatLooksLikeAMarkerComesBackAsItWas(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	translated, err := translator.Translate(context.Background(),
		"Die Notation ⟦0⟧ kommt aus der Mathematik.")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if !strings.Contains(translated, "⟦0⟧") {
		t.Errorf("Translate returned %q, want the notation kept", translated)
	}
}

// Something marker-shaped inside a protected span used to be enough to make the
// second marker look repeated: the span was put back first, and what it carried
// was then counted as a marker of its own. Every marker carries a nonce drawn
// for this translation, so the two cannot be taken for each other.
func TestSomethingMarkerShapedInsideASpanIsNotMistakenForAMarker(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	draft := "Die Notation `⟦1⟧` und `berechneRabatt()` stehen in der Doku."
	translated, err := translator.Translate(context.Background(), draft)
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	for _, span := range []string{"`⟦1⟧`", "`berechneRabatt()`"} {
		if !strings.Contains(translated, span) {
			t.Errorf("Translate returned %q, want %s back untouched", translated, span)
		}
	}
}

// Live mode and the allowance counter both look through wrappers.
func TestProtectingCanBeLookedThrough(t *testing.T) {
	t.Parallel()
	service := &reportingSpy{spent: translation.Usage{Used: 12, Limit: 500}}

	reporter, err := translation.ReporterOf(translation.Protecting(translation.Segmented(service)))
	if err != nil {
		t.Fatalf("ReporterOf returned unexpected error: %v", err)
	}
	spent, err := reporter.Usage(context.Background())
	if err != nil {
		t.Fatalf("Usage returned unexpected error: %v", err)
	}
	if spent.Used != 12 {
		t.Errorf("Usage returned %+v, want what the service said", spent)
	}
}
