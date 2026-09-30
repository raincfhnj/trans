package setup

import (
	"encoding/json"
	"fmt"
	"io"
)

// Verdict is what one check concluded.
type Verdict string

const (
	// Pass says the thing is there and can be relied on.
	Pass Verdict = "PASS"
	// Warn says the thing is missing or doubtful, but the panel still works
	// without it — the line carries what to do about it.
	Warn Verdict = "WARN"
	// Fail says the thing is required and is not right; the fix belongs in the
	// line that carries it.
	Fail Verdict = "FAIL"
)

// Check is one line of the report: what was looked at, what came of it, and —
// when it did not come out right — what a person should do about it.
type Check struct {
	Name    string  `json:"name"`
	Verdict Verdict `json:"verdict"`
	// Detail is what the check found, in the plainest words that fit.
	Detail string `json:"detail"`
	// Fix is the action that would turn a WARN or a FAIL into a PASS. It is
	// empty when there is nothing to do.
	Fix string `json:"fix,omitempty"`
}

// Report is the whole walk: the checks in the order they were run, and the
// counts that say how it went without reading the lines.
type Report struct {
	Checks []Check `json:"checks"`
	Pass   int     `json:"pass"`
	Warn   int     `json:"warn"`
	Fail   int     `json:"fail"`
}

// Add appends a check and keeps the counts in step.
func (r *Report) Add(check Check) {
	r.Checks = append(r.Checks, check)
	switch check.Verdict {
	case Pass:
		r.Pass++
	case Warn:
		r.Warn++
	case Fail:
		r.Fail++
	}
}

// Healthy says nothing failed. Warnings are things that could be better; only
// a failure keeps the setup from working.
func (r Report) Healthy() bool {
	return r.Fail == 0
}

// WriteHuman draws the report as the lines a person reads at a terminal: a
// verdict, the check, and what was found — with the fix on its own line when
// there is one.
func (r Report) WriteHuman(out io.Writer) error {
	for _, check := range r.Checks {
		if _, err := fmt.Fprintf(out, "%-4s %s: %s\n", check.Verdict, check.Name, check.Detail); err != nil {
			return err
		}
		if check.Fix != "" {
			if _, err := fmt.Fprintf(out, "     fix: %s\n", check.Fix); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(out, "\n%d passed, %d warnings, %d failures\n", r.Pass, r.Warn, r.Fail)
	return err
}

// WriteJSON draws the report as one machine-readable object, indented so a
// person can read it too.
func (r Report) WriteJSON(out io.Writer) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}
