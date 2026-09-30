package setup_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"trans/internal/config"
	"trans/internal/service"
	"trans/internal/setup"
)

// The two programs are convenience, not necessity: both there passes, one
// there is a narrower warning, neither is the wider one — never a failure.
func TestProgramsOnPathAreWarnedAboutRatherThanFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		onPath  map[string]string
		verdict setup.Verdict
		inBoth  []string
	}{
		{
			name: "both programs are found",
			onPath: map[string]string{
				"trans-window.exe":  `C:\bin\trans-window.exe`,
				"trans-windowd.exe": `C:\bin\trans-windowd.exe`,
			},
			verdict: setup.Pass,
			inBoth:  []string{`C:\bin\trans-window.exe`, `C:\bin\trans-windowd.exe`},
		},
		{
			name: "only the panel is found",
			onPath: map[string]string{
				"trans-window.exe": `C:\bin\trans-window.exe`,
			},
			verdict: setup.Warn,
			inBoth:  []string{`C:\bin\trans-window.exe`, "trans-windowd.exe is not on PATH"},
		},
		{
			name:    "neither is found",
			onPath:  map[string]string{},
			verdict: setup.Warn,
			inBoth:  []string{"neither", "make windows"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			file := aSettingsFile(t, "")
			report := setup.Doctor((&fake{
				settings: config.Settings{ConfigFile: file},
				choice:   service.Choice{Name: "off", Translates: false},
				onPath:   test.onPath,
			}).options())

			check := checkNamed(t, report, "programs on PATH")
			if check.Verdict != test.verdict {
				t.Errorf("verdict is %q, want %q", check.Verdict, test.verdict)
			}
			if check.Verdict == setup.Fail {
				t.Error("missing programs must not fail the setup")
			}
			for _, wanted := range test.inBoth {
				if !strings.Contains(check.Detail+" "+check.Fix, wanted) {
					t.Errorf("line %q / fix %q does not contain %q", check.Detail, check.Fix, wanted)
				}
			}
		})
	}
}

// Speech is asked for best-effort and never fails the setup: the panel has no
// audio dependency of its own, so a voice that cannot be listed is a warning
// with where to install one, and a platform that was not asked says so.
func TestSpeechIsBestEffortAndNeverFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		speech     string
		hasSpeech  bool
		asked      bool
		verdict    setup.Verdict
		wantDetail []string
		wantFix    string
	}{
		{
			name:       "a default voice is available",
			speech:     "Microsoft Zira Desktop",
			hasSpeech:  true,
			asked:      true,
			verdict:    setup.Pass,
			wantDetail: []string{"Microsoft Zira Desktop", "Windows SAPI"},
		},
		{
			name:       "no voice could be listed",
			asked:      true,
			verdict:    setup.Warn,
			wantDetail: []string{"SAPI"},
			wantFix:    "Speech",
		},
		{
			name:       "the platform was not asked",
			verdict:    setup.Warn,
			wantDetail: []string{"testos"},
			wantFix:    "optional",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			file := aSettingsFile(t, "")
			options := (&fake{
				settings:  config.Settings{ConfigFile: file},
				choice:    service.Choice{Name: "off", Translates: false},
				speech:    test.speech,
				hasSpeech: test.hasSpeech,
			}).options()
			if !test.asked {
				options.Speech = nil
			}

			report := setup.Doctor(options)
			check := checkNamed(t, report, "speech synthesis")
			if check.Verdict != test.verdict {
				t.Errorf("verdict is %q, want %q", check.Verdict, test.verdict)
			}
			if check.Verdict == setup.Fail {
				t.Error("speech must never fail the setup")
			}
			for _, wanted := range test.wantDetail {
				if !strings.Contains(check.Detail, wanted) {
					t.Errorf("detail %q does not contain %q", check.Detail, wanted)
				}
			}
			if test.wantFix != "" && !strings.Contains(check.Fix, test.wantFix) {
				t.Errorf("fix %q does not contain %q", check.Fix, test.wantFix)
			}
		})
	}
}

// The human output is the lines a person reads: verdict, name, finding, and
// the fix underneath when there is one, closed by the counts.
func TestHumanOutputIsTheLinesAPersonReads(t *testing.T) {
	t.Parallel()

	var report setup.Report
	report.Add(setup.Check{Name: "settings", Verdict: setup.Pass, Detail: "/cfg/.env (42 bytes)"})
	report.Add(setup.Check{
		Name:    "provider key",
		Verdict: setup.Warn,
		Detail:  "no service is configured",
		Fix:     "set TRANS_PROVIDER",
	})

	var drawn bytes.Buffer
	if err := report.WriteHuman(&drawn); err != nil {
		t.Fatalf("WriteHuman: %v", err)
	}
	output := drawn.String()

	wanted := []string{
		"PASS settings: /cfg/.env (42 bytes)",
		"WARN provider key: no service is configured",
		"     fix: set TRANS_PROVIDER",
		"1 passed, 1 warnings, 0 failures",
	}
	for _, line := range wanted {
		if !strings.Contains(output, line) {
			t.Errorf("output does not contain %q:\n%s", line, output)
		}
	}
}

// The JSON output is the same report as an object: a machine reads the counts
// and the checks without having to parse the drawing.
func TestJSONOutputCarriesTheCountsAndEveryCheck(t *testing.T) {
	t.Parallel()

	var report setup.Report
	report.Add(setup.Check{Name: "settings", Verdict: setup.Pass, Detail: "/cfg/.env"})
	report.Add(setup.Check{Name: "speech synthesis", Verdict: setup.Fail, Detail: "no voice", Fix: "install one"})

	var drawn bytes.Buffer
	if err := report.WriteJSON(&drawn); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var decoded struct {
		Pass   int `json:"pass"`
		Warn   int `json:"warn"`
		Fail   int `json:"fail"`
		Checks []struct {
			Name    string `json:"name"`
			Verdict string `json:"verdict"`
			Detail  string `json:"detail"`
			Fix     string `json:"fix"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(drawn.Bytes(), &decoded); err != nil {
		t.Fatalf("decoding the output %q: %v", drawn.String(), err)
	}

	if decoded.Pass != 1 || decoded.Fail != 1 {
		t.Errorf("counts are pass=%d fail=%d, want 1 and 1", decoded.Pass, decoded.Fail)
	}
	if len(decoded.Checks) != 2 {
		t.Fatalf("got %d checks, want 2", len(decoded.Checks))
	}
	if decoded.Checks[1].Name != "speech synthesis" || decoded.Checks[1].Verdict != "FAIL" {
		t.Errorf("second check is %+v", decoded.Checks[1])
	}
	if decoded.Checks[1].Fix != "install one" {
		t.Errorf("fix is %q, want it carried through", decoded.Checks[1].Fix)
	}
	// A check with nothing to do omits the fix rather than sending an empty one.
	if strings.Contains(drawn.String(), `"fix":""`) {
		t.Error("an empty fix is printed instead of omitted")
	}
}
