package translation

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Services translate code as if it were prose: identifiers get renamed, comments
// and string literals are rewritten, and indentation is reformatted. Code is
// therefore never sent. A marker stands in its place, which keeps the sentence
// around it whole — a service given only the prose between the code would be
// translating fragments.
//
// A marker is a bracket, a nonce drawn fresh for this translation, and a number.
// The nonce is what keeps a draft that itself contains something shaped like a
// marker — `⟦0⟧` in a sentence about notation — from being taken for one of ours
// on the way back: the draft was written before this nonce existed, so nothing
// in it can carry it. The brackets stay because nobody types them by accident
// and a service leaves them alone; the digits alone would not do, because a
// draft can carry those.
const (
	markerOpen  = "⟦"
	markerClose = "⟧"
)

// A fence runs to the closing one, or to the end of a draft still being written.
// An inline span follows the same rule: it needs both backticks to close — and
// may cross a line break — while a lone one, still being typed, holds back
// everything after it, so half-written code is never sent.
var protectedSpans = regexp.MustCompile("(?s)```.*?```|```.*$|`[^`]+`|`[^`]*$")

// nonces counts the drawings in this process, so two markers made in the same
// nanosecond are still different from each other.
var nonces atomic.Uint64

type protecting struct{ translator Translator }

// Protecting keeps code out of a translation, and out of the request.
func Protecting(translator Translator) Translator {
	return &protecting{translator: translator}
}

func (p *protecting) Translate(ctx context.Context, draft string) (string, error) {
	nonce := newNonce()
	marked, protected := takeOut(draft, nonce)
	if len(protected) == 0 {
		return p.translator.Translate(ctx, draft)
	}

	translated, err := p.translator.Translate(ctx, marked)
	if err != nil {
		return "", err
	}
	return putBack(translated, protected, nonce)
}

func (p *protecting) Unwrap() Translator { return p.translator }

// takeOut replaces every protected span with a marker, and says what was taken.
func takeOut(draft, nonce string) (marked string, protected []string) {
	marked = protectedSpans.ReplaceAllStringFunc(draft, func(span string) string {
		protected = append(protected, span)
		return marker(nonce, len(protected)-1)
	})
	return marked, protected
}

// putBack restores what was taken out. A service that dropped or repeated a
// marker has lost or duplicated the code with it, which is worth saying rather
// than handing an agent a prompt with a hole in it.
func putBack(translated string, protected []string, nonce string) (string, error) {
	for index, span := range protected {
		found := marker(nonce, index)
		switch strings.Count(translated, found) {
		case 1:
			translated = strings.Replace(translated, found, span, 1)
		case 0:
			return "", errors.New("the service did not return a protected part of the draft")
		default:
			return "", errors.New("the service repeated a protected part of the draft")
		}
	}
	return translated, nil
}

// newNonce is unique among the nonces this process draws: the clock alone would
// repeat itself twice in the same nanosecond, and the counter alone would repeat
// itself after a restart with a draft that outlived it.
func newNonce() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(nonces.Add(1), 36)
}

func marker(nonce string, index int) string {
	return markerOpen + nonce + "-" + strconv.Itoa(index) + markerClose
}
