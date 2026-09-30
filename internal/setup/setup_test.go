package setup_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trans/internal/config"
	"trans/internal/service"
	"trans/internal/setup"
)

// fake builds the injected checkers a test needs: settings read from a map, a
// choice of service, a PATH that holds exactly the programs named, and a
// speech answer that is whatever the test says.
type fake struct {
	env       map[string]string
	settings  config.Settings
	loadErr   error
	choice    service.Choice
	onPath    map[string]string
	speech    string
	hasSpeech bool
}

func (f *fake) options() setup.Options {
	choose := func(*config.Settings) service.Choice { return f.choice }
	return setup.Options{
		Getenv: func(key string) string { return f.env[key] },
		Load: func(func(string) string) (config.Settings, error) {
			if f.loadErr != nil {
				return config.Settings{}, f.loadErr
			}
			return f.settings, nil
		},
		Choose: choose,
		LookPath: func(name string) (string, error) {
			if path, found := f.onPath[name]; found {
				return path, nil
			}
			return "", errors.New(name + " is not on PATH")
		},
		Speech: func() (string, bool) { return f.speech, f.hasSpeech },
		// The doctor reports the platform; only the speech check reads it.
		Operating: "testos",
	}
}

// aSettingsFile gives the fake a real file to report the size of, which is
// the only thing the settings check does with the disk.
func aSettingsFile(t *testing.T, contents string) string {
	t.Helper()
	directory := t.TempDir()
	file := filepath.Join(directory, ".env")
	if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing the settings file: %v", err)
	}
	return file
}

func checkNamed(t *testing.T, report setup.Report, name string) setup.Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("no check named %q in %+v", name, report.Checks)
	return setup.Check{}
}

// The report keeps every check in order and counts them in step: both output
// modes read those numbers, so they are asserted first and generally.
func TestReportKeepsEveryCheckAndCountsThem(t *testing.T) {
	t.Parallel()

	file := aSettingsFile(t, "TRANS_PROVIDER=gtranslate\n")
	report := setup.Doctor((&fake{
		settings:  config.Settings{ConfigFile: file},
		choice:    service.Choice{Name: "gtranslate", Translates: true},
		onPath:    map[string]string{"go": `C:\go\bin\go.exe`},
		speech:    "Microsoft Zira Desktop",
		hasSpeech: true,
	}).options())

	if len(report.Checks) != 5 {
		t.Fatalf("got %d checks, want 5: %+v", len(report.Checks), report.Checks)
	}
	if total := report.Pass + report.Warn + report.Fail; total != len(report.Checks) {
		t.Errorf("counts are %d+%d+%d=%d for %d checks",
			report.Pass, report.Warn, report.Fail, total, len(report.Checks))
	}
	if report.Fail > 0 && report.Healthy() {
		t.Error("Healthy says the setup is fine while something failed")
	}
	if report.Fail == 0 && !report.Healthy() {
		t.Error("Healthy says the setup is broken while nothing failed")
	}
}

// A toolchain that is not there is a warning with an install to reach for,
// never a failure: prebuilt installations do not need a compiler.
func TestGoToolchainIsWarnedAboutRatherThanFailed(t *testing.T) {
	t.Parallel()

	file := aSettingsFile(t, "")
	report := setup.Doctor((&fake{
		settings: config.Settings{ConfigFile: file},
		choice:   service.Choice{Name: "off", Translates: false},
	}).options())

	check := checkNamed(t, report, "go toolchain")
	if check.Verdict != setup.Warn {
		t.Errorf("verdict is %q, want WARN", check.Verdict)
	}
	if !strings.Contains(check.Fix, "go.dev") {
		t.Errorf("fix is %q, want an install to reach for", check.Fix)
	}
}

// The settings line follows what the panel does: config.Prepare first, then
// config.Load. A file that exists passes with its size; a refusal fails with
// the reader's own message and something to do about it.
func TestSettingsCheckFollowsPrepareAndLoad(t *testing.T) {
	t.Parallel()

	t.Run("a readable file passes with its size", func(t *testing.T) {
		t.Parallel()
		file := aSettingsFile(t, "TRANS_PROVIDER=gtranslate\n")
		report := setup.Doctor((&fake{
			settings: config.Settings{ConfigFile: file},
			choice:   service.Choice{Name: "gtranslate", Translates: true},
		}).options())

		check := checkNamed(t, report, "settings")
		if check.Verdict != setup.Pass {
			t.Errorf("verdict is %q, want PASS", check.Verdict)
		}
		if !strings.Contains(check.Detail, file) {
			t.Errorf("detail %q does not name the file %q", check.Detail, file)
		}
		if !strings.Contains(check.Detail, "bytes") {
			t.Errorf("detail %q does not report the size", check.Detail)
		}
	})

	t.Run("no config directory is a warning", func(t *testing.T) {
		t.Parallel()
		report := setup.Doctor((&fake{
			choice: service.Choice{Name: "off", Translates: false},
		}).options())

		check := checkNamed(t, report, "settings")
		if check.Verdict != setup.Warn {
			t.Errorf("verdict is %q, want WARN", check.Verdict)
		}
		if !strings.Contains(check.Fix, "TRANS_CONFIG_DIR") {
			t.Errorf("fix %q does not name the variable", check.Fix)
		}
	})

	t.Run("settings that cannot be read fail with what to correct", func(t *testing.T) {
		t.Parallel()
		report := setup.Doctor((&fake{
			loadErr: errors.New(`TRANS_SUBMIT is "maybe", which is neither on nor off`),
		}).options())

		check := checkNamed(t, report, "settings")
		if check.Verdict != setup.Fail {
			t.Errorf("verdict is %q, want FAIL", check.Verdict)
		}
		if !strings.Contains(check.Detail, "TRANS_SUBMIT") {
			t.Errorf("detail %q does not carry the reader's message", check.Detail)
		}
		if check.Fix == "" {
			t.Error("a failure must carry a fix")
		}
	})
}

// The key line is where a credential could leak: it names the variable and
// never the value, whatever the service is and however the key arrived.
func TestProviderKeyIsReportedWithoutBeingPrinted(t *testing.T) {
	t.Parallel()

	const secret = "sk-super-secret-value-1234"

	tests := []struct {
		name       string
		provider   string
		key        string
		choice     service.Choice
		verdict    setup.Verdict
		wantDetail []string
		fixHas     string
	}{
		{
			name:       "a key is masked to its last four characters",
			provider:   "openai",
			key:        secret,
			choice:     service.Choice{Name: "openai", Translates: true},
			verdict:    setup.Pass,
			wantDetail: []string{"openai", "TRANS_OPENAI_API_KEY", "1234"},
		},
		{
			name:       "no service chosen is a warning with both ways out",
			choice:     service.Choice{Name: "off", Translates: false},
			verdict:    setup.Warn,
			wantDetail: []string{"draft box"},
			fixHas:     "TRANS_PROVIDER",
		},
		{
			name:       "a service that could not be built fails and says where to configure it",
			provider:   "deepl",
			choice:     service.Choice{Name: "deepl", Translates: true, Trouble: errors.New("deepl: no key")},
			verdict:    setup.Fail,
			wantDetail: []string{"deepl: no key"},
			fixHas:     ".env",
		},
		{
			name:       "a service that needs no key passes without one",
			provider:   "gtranslate",
			choice:     service.Choice{Name: "gtranslate", Translates: true},
			verdict:    setup.Pass,
			wantDetail: []string{"gtranslate", "needs no key"},
		},
		{
			name:       "an unscoped key names the generic variable",
			key:        secret,
			choice:     service.Choice{Name: "deepl", Translates: true},
			verdict:    setup.Pass,
			wantDetail: []string{"TRANS_API_KEY", "1234"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			file := aSettingsFile(t, "")
			settings := config.Settings{Provider: test.provider, ConfigFile: file}
			settings.Options.APIKey = test.key

			report := setup.Doctor((&fake{
				settings: settings,
				choice:   test.choice,
			}).options())

			check := checkNamed(t, report, "provider key")
			if check.Verdict != test.verdict {
				t.Errorf("verdict is %q, want %q (detail %q)", check.Verdict, test.verdict, check.Detail)
			}
			for _, wanted := range test.wantDetail {
				if !strings.Contains(check.Detail, wanted) {
					t.Errorf("detail %q does not contain %q", check.Detail, wanted)
				}
			}
			if strings.Contains(check.Detail, secret) {
				t.Errorf("detail %q prints the key", check.Detail)
			}
			if test.fixHas != "" && !strings.Contains(check.Fix, test.fixHas) {
				t.Errorf("fix %q does not contain %q", check.Fix, test.fixHas)
			}
		})
	}
}
