// Command trans opens an overlay for composing a prompt in your own
// language and delivers it to a herdr agent in the target language.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"trans/internal/config"
	"trans/internal/draft"
	"trans/internal/herdr"
	"trans/internal/overlay"
	"trans/internal/promptflow"
	"trans/internal/service"
	"trans/internal/translation"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "open":
			if err := runOpen(context.Background(), os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "herdr-trans:", err)
				os.Exit(1)
			}
			return

		// warm does nothing but exist: running it puts the binary in the page
		// cache, so the keypress that opens the popup does not pay for that.
		case "warm":
			return
		}
	}

	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "herdr-trans:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	settings, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	chosen := service.Choose(&settings)
	translator := chosen.Translator

	// Herdr closes a popup by hanging up on it. Ending on the signal instead of
	// dying on it is what lets the draft be kept below.
	ctx, stopListening := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stopListening()

	runner := herdr.NewExecRunner(settings.HerdrBinary)
	flowOptions := []promptflow.Option{
		// Writing means translating the same draft again and again, so a preview
		// pays for each sentence once. Live translation can be switched on at any
		// time, so this is set up whether it starts on or off. Protecting sits
		// outside the cache, so a fenced block is taken out before the draft is
		// split into sentences.
		promptflow.WithPreviewTranslator(
			translation.Protecting(translation.Segmented(translator))),
	}
	if spending, keepsCount := service.UsageReporter(translator); keepsCount {
		flowOptions = append(flowOptions, promptflow.WithUsageReporter(spending))
	}

	flow := promptflow.New(translation.Protecting(translator),
		herdr.NewAgentPrompt(runner, settings.Target),
		herdr.NewPaneText(runner, settings.Target),
		flowOptions...,
	)
	program := tea.NewProgram(
		overlay.New(ctx, flow, overlay.Options{
			Service:        chosen.Name,
			WithoutService: !chosen.Translates,
			Trouble:        chosen.Trouble,
			Language:       settings.Options.TargetLanguage,
			Review:         !settings.Submit,
			Vim:            settings.Vim,
			Live:           settings.Live,
			Confirm:        settings.Confirm,
			Pulse:          settings.Pulse,
			Logo:           settings.Logo,
			MaxDraft:       settings.MaxDraft,

			Drafts: drafts(settings.KeepDraft, settings.StateDir, settings.Target),
		}),
		tea.WithContext(ctx),
		// No alternate screen: entering it clears the pane to the terminal's own
		// background, which then differs from the popup herdr painted around it.
		// Drawing inline leaves every cell we do not write as herdr left it.
	)
	final, err := program.Run()
	if kept, ok := final.(overlay.Model); ok {
		if err := kept.KeepUnfinished(); err != nil {
			fmt.Fprintln(os.Stderr, "trans:", err)
		}
	}
	// A closed popup ends the program this way; it is not a failure.
	if err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fmt.Errorf("running overlay: %w", err)
	}
	return nil
}

// drafts keeps an unfinished prompt for this pane, unless the author would
// rather start from an empty box every time.
func drafts(keep bool, stateDir, target string) overlay.Drafts {
	if !keep || stateDir == "" {
		return nil
	}
	return draft.NewStore(stateDir).For(target)
}
