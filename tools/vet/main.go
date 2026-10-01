// Command vet runs `go vet` over the tree the way this repository wants it
// run: every package checked as it stands, except internal/win32, where the two
// conversions that turn an address Windows handed back into a pointer are
// exactly the pattern vet's unsafeptr check cannot be told is safe.
//
// The exception is made here rather than in the Makefile so that `make vet`
// needs no grep, sed or shell substitution next to it: the same gate then runs
// on Windows, Linux and macOS, and in CI. It is one package, not the whole
// tree: everywhere else unsafeptr stays on.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// exempt is the package whose unsafeptr findings are its own business, with the
// reason: Win32 hands addresses back as numbers, and there is no way to read
// what they point at without converting one.
const exempt = "trans/internal/win32"

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "vet:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	packages, err := list(ctx)
	if err != nil {
		return err
	}

	checked := make([]string, 0, len(packages))
	skipped := false
	for _, path := range packages {
		if path == exempt {
			skipped = true
			continue
		}
		checked = append(checked, path)
	}

	if len(checked) > 0 {
		if err := vet(ctx, checked...); err != nil {
			return err
		}
	}
	if !skipped {
		// The package was renamed or removed: the exception is stale, and a
		// stale exception is how a gate quietly stops checking anything.
		return fmt.Errorf("%s is not in this module any more; drop the exception", exempt)
	}
	// unsafeptr stays off for this one package only; every other check runs.
	return vet(ctx, "-unsafeptr=false", "./internal/win32/")
}

// list is every package in the module, the main programs included.
//
// #nosec G204 — the arguments are this program's own, and the only variable
// one is a package path `go list` itself just printed.
func list(ctx context.Context) ([]string, error) {
	output, err := exec.CommandContext(ctx, "go", "list", "./...").Output()
	if err != nil {
		return nil, fmt.Errorf("listing the packages: %w", err)
	}
	var packages []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			packages = append(packages, line)
		}
	}
	if len(packages) == 0 {
		return nil, errors.New("no packages to check")
	}
	return packages, nil
}

// vet runs the tool with its output and its failure going straight to the
// terminal, so a finding reads exactly as `go vet` would print it.
//
// #nosec G204 — the program is `go` itself and every argument is either a
// literal written above or a package path `go list` just printed.
func vet(ctx context.Context, arguments ...string) error {
	command := exec.CommandContext(ctx, "go", append([]string{"vet"}, arguments...)...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}
