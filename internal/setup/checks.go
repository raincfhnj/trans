package setup

import (
	"fmt"
	"os"
	"strings"

	"trans/internal/config"
	"trans/internal/service"
)

// Options are the seams the doctor is driven through: every one of them has a
// real implementation for a person at a terminal and a fake for the tests, so
// the checks themselves never need a machine, a network or a PATH.
type Options struct {
	// Getenv reads the environment the settings are read from.
	Getenv func(string) string
	// Load is config.Load: the settings as the panel would read them.
	Load func(getenv func(string) string) (config.Settings, error)
	// Choose is service.Choose: which translation service those settings pick.
	Choose func(*config.Settings) service.Choice
	// LookPath reports where a program on PATH is, or that it is not there.
	LookPath func(string) (string, error)
	// Speech reports the default speech-synthesis voice, if one can be listed.
	// A nil one means the platform's answer was not asked for.
	Speech func() (string, bool)
	// Operating is runtime.GOOS, for the checks that only make sense somewhere.
	Operating string
}

// Doctor walks the checks in the order a person would ask them and returns
// everything it found. It never fails: a check that could not be run is
// itself reported as a line.
func Doctor(options Options) Report {
	var report Report

	report.Add(goToolchain(options))
	report.Add(configuration(options))
	report.Add(credentials(options))
	report.Add(binaries(options))
	report.Add(speech(options))
	return report
}

// goToolchain is the compiler the panel itself is built with. It only matters
// to someone building from source; an installation that runs prebuilt
// binaries does not need it, which is why its absence is a warning rather
// than a failure.
func goToolchain(options Options) Check {
	const name = "go toolchain"

	found, err := options.LookPath("go")
	if err != nil {
		return Check{
			Name:    name,
			Verdict: Warn,
			Detail:  "go is not on PATH",
			Fix:     "install Go from https://go.dev/dl/ to build trans from source",
		}
	}
	return Check{Name: name, Verdict: Pass, Detail: found}
}

// configuration is where the settings live and whether what is there can be
// read. Prepare is what the panel itself calls first, so calling it here
// reports the file exactly as the panel would see it.
func configuration(options Options) Check {
	const name = "settings"

	config.Prepare()

	settings, err := options.Load(options.Getenv)
	if err != nil {
		return Check{
			Name:    name,
			Verdict: Fail,
			Detail:  fmt.Sprintf("the settings could not be read: %v", err),
			Fix:     fixForLoad(err),
		}
	}

	file := settings.ConfigFile
	if file == "" {
		return Check{
			Name:    name,
			Verdict: Warn,
			Detail:  "no config directory, so no settings file is read",
			Fix:     "set TRANS_CONFIG_DIR to a directory the panel may write",
		}
	}
	if info, statErr := os.Stat(file); statErr == nil {
		return Check{
			Name:    name,
			Verdict: Pass,
			Detail:  fmt.Sprintf("%s (%d bytes)", file, info.Size()),
		}
	}
	// Prepare has just written the starter file, so a miss here means the
	// directory itself could not be made.
	return Check{
		Name:    name,
		Verdict: Warn,
		Detail:  fmt.Sprintf("no settings file yet at %s", file),
		Fix:     "run the panel once, or write the settings there by hand",
	}
}

// fixForLoad turns the settings reader's refusal into the action that
// resolves it: the message already names the variable and its value.
func fixForLoad(err error) string {
	return fmt.Sprintf("correct the setting it names, then run setup again (%v)", err)
}

// credentials is the key the chosen service would send. The key itself is
// never printed — only the variable it should live in and, at most, its last
// four characters, so two keys can be told apart without either being shown.
func credentials(options Options) Check {
	const name = "provider key"

	settings, err := options.Load(options.Getenv)
	if err != nil {
		return Check{
			Name:    name,
			Verdict: Fail,
			Detail:  "the settings could not be read, so no service was chosen",
			Fix:     fixForLoad(err),
		}
	}

	chosen := options.Choose(&settings)
	if chosen.Trouble != nil {
		return Check{
			Name:    name,
			Verdict: Fail,
			Detail:  chosen.Trouble.Error(),
			Fix:     fmt.Sprintf("set the service or its key in %s", settings.ConfigFile),
		}
	}
	if !chosen.Translates {
		return Check{
			Name:    name,
			Verdict: Warn,
			Detail:  "no service is configured, so the panel is a plain draft box",
			Fix:     "set TRANS_PROVIDER (gtranslate needs no key) or TRANS_API_KEY",
		}
	}

	keyVariable := scopedKey(settings.Provider)
	if settings.Options.APIKey == "" {
		// A service that needs no key still passes: choosing it was the
		// configuration, and it translates without one.
		return Check{
			Name:    name,
			Verdict: Pass,
			Detail:  fmt.Sprintf("%s is chosen and needs no key", chosen.Name),
		}
	}
	return Check{
		Name:    name,
		Verdict: Pass,
		Detail: fmt.Sprintf("%s is chosen; %s is set (%s)",
			chosen.Name, keyVariable, mask(settings.Options.APIKey)),
	}
}

// scopedKey is the variable a key for one service is read from, spelled the
// way config spells it.
func scopedKey(provider string) string {
	if provider == "" {
		return "TRANS_API_KEY"
	}
	scope := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(provider))
	return "TRANS_" + scope + "_API_KEY"
}

// binaries are the two programs a person types at a terminal. Neither is
// required — the panel is opened by the daemon or by a full path — so both
// missing is a warning with the command that would put them there.
func binaries(options Options) Check {
	const name = "programs on PATH"

	var found []string
	for _, program := range []string{"trans-window.exe", "trans-windowd.exe"} {
		if path, err := options.LookPath(program); err == nil {
			found = append(found, path)
		}
	}

	switch len(found) {
	case 2:
		return Check{Name: name, Verdict: Pass, Detail: strings.Join(found, ", ")}
	case 1:
		return Check{
			Name:    name,
			Verdict: Warn,
			Detail:  fmt.Sprintf("found %s; trans-windowd.exe is not on PATH", found[0]),
			Fix:     "add the directory holding both programs to PATH",
		}
	default:
		return Check{
			Name:    name,
			Verdict: Warn,
			Detail:  "neither trans-window.exe nor trans-windowd.exe is on PATH",
			Fix:     "run `make windows` and add bin\\ to PATH, or call the programs by full path",
		}
	}
}

// speech is Windows' own voice, asked for best-effort: the panel has no audio
// dependency of its own, so this reports what the machine can do and never
// fails the setup over it.
func speech(options Options) Check {
	const name = "speech synthesis"

	if options.Speech == nil {
		return Check{
			Name:    name,
			Verdict: Warn,
			Detail:  fmt.Sprintf("speech synthesis was not checked on %s", options.Operating),
			Fix:     "the panel itself speaks no audio; Windows SAPI voices are optional",
		}
	}
	voice, ok := options.Speech()
	if !ok {
		return Check{
			Name:    name,
			Verdict: Warn,
			Detail:  "no default speech voice could be listed (Windows SAPI / System.Speech)",
			Fix:     "install a voice in Windows Settings > Time & Language > Speech",
		}
	}
	return Check{
		Name:    name,
		Verdict: Pass,
		Detail:  fmt.Sprintf("default voice %q is available via Windows SAPI", voice),
	}
}

// mask keeps a credential out of the report. The last four characters say
// which key it is; the rest is never printed, in either output mode.
func mask(secret string) string {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 4 {
		return "****"
	}
	return "****" + trimmed[len(trimmed)-4:]
}
