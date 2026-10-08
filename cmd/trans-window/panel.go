//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"trans/internal/config"
	"trans/internal/overlay"
	"trans/internal/promptflow"
	"trans/internal/service"
	"trans/internal/translation"
	"trans/internal/win32"
	"trans/internal/winlog"

	tea "github.com/charmbracelet/bubbletea"
)

// panelDrawing is what a host says about the box it draws: what it opens with,
// where the finished prompt goes, and how much room there is. The box itself
// is the same in every host �� the popup over a window and the TUI in a
// terminal differ in their frames, not in their draft.
type panelDrawing struct {
	// read turns the box around: English in, the author's language back.
	read bool
	// review says the send key only types the prompt in.
	review bool
	// prefill is what the box opens with, and prefillTrouble is why there is
	// nothing when something was asked for and did not come.
	prefill        string
	prefillTrouble error
	// trouble is what the host could not work out before the box opened ��� a
	// window named that is not there ��� said as loudly as a broken service.
	trouble error
	// slot names the pane's own kept draft and history: the window the prompt
	// is delivered into, by the title the author knows it by.
	slot string
	// width is how many columns the host can draw.
	width int
	// cursor is told where the caret sits, so a terminal drawing its own
	// cursor there shows an input method's pre-edit where the writing is.
	cursor *overlay.CursorPlace
}

// panelService picks the service a draft goes through. Read mode is the same
// service pointed the other way: same provider, same credentials, same bill.
func panelService(cfg *config.Settings, read bool) service.Choice {
	if read {
		cfg.Options.TargetLanguage = cfg.ReadLanguage
	}
	return service.Choose(cfg)
}

// panelFlow wires the draft's two ways out: the service it is translated with,
// and where the finished prompt is delivered. Writing means translating the
// same draft again and again, so the preview goes through a sentence cache
// that pays for each sentence once; Protecting sits outside it, so a fenced
// block is taken out before the draft is split into sentences.
//
// Read mode delivers nowhere: what comes back is copied, and the author pastes
// it wherever it belongs.
func panelFlow(chosen service.Choice, sending, typing promptflow.Target, read bool) *promptflow.Flow {
	translator := chosen.Translator
	flowOptions := []promptflow.Option{
		promptflow.WithPreviewTranslator(
			translation.Protecting(translation.Segmented(translator))),
	}
	if spending, keepsCount := service.UsageReporter(translator); keepsCount {
		flowOptions = append(flowOptions, promptflow.WithUsageReporter(spending))
	}
	if read {
		sending, typing = clipboardTarget{}, clipboardTarget{}
	}
	return promptflow.New(translation.Protecting(translator), sending, typing, flowOptions...)
}

// panelModel builds the draft box every host draws.
func panelModel(ctx context.Context, cfg *config.Settings, chosen service.Choice,
	flow *promptflow.Flow, drawing *panelDrawing,
) overlay.Model {
	return overlay.New(ctx, flow, overlay.Options{
		Service:        chosen.Name,
		WithoutService: !chosen.Translates,
		Trouble:        errors.Join(chosen.Trouble, drawing.trouble),
		Language:       cfg.Options.TargetLanguage,
		Review:         drawing.review,
		Vim:            cfg.Vim,
		Live:           cfg.Live,
		Confirm:        cfg.Confirm,
		Pulse:          cfg.Pulse,
		Logo:           cfg.Logo,
		MaxDraft:       cfg.MaxDraft,
		// Windows Terminal opens alt+enter full screen, so the panel says the
		// key that does send: ctrl+d.
		SendKey: "ctrl+d",
		Cursor:  drawing.cursor,

		Read:           drawing.read,
		Prefill:        drawing.prefill,
		PrefillTrouble: drawing.prefillTrouble,

		Drafts:  drafts(cfg, drawing.slot),
		History: historyLog(cfg, drawing.slot),

		// The colour scheme and the box's size are chosen in the settings, so
		// they reach the panel the same way the rest of the options do; the
		// window that opens next is the one they take effect in.
		Theme:      cfg.Theme,
		DraftRows:  cfg.DraftRows,
		PanelWidth: drawing.width,
	})
}

// runOverlay draws the box and keeps whatever was left unfinished in it. A
// window that was closed ends the program by hanging up on it; that is the
// program's own end and not a failure.
func runOverlay(program *tea.Program, logName, failing string) error {
	final, err := program.Run()
	if kept, ok := final.(overlay.Model); ok {
		if keepErr := kept.KeepUnfinished(); keepErr != nil {
			fmt.Fprintln(os.Stderr, "trans-window:", keepErr)
		}
	}
	winlog.Notef(logName, "closed: %v", err)
	if err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fmt.Errorf("%s: %w", failing, err)
	}
	return nil
}

// clipboardTarget is where a prompt goes when there is no window to deliver it
// into: the author pastes it wherever it belongs. Read mode uses the same one
// on purpose �� its result is never delivered into a window either.
type clipboardTarget struct{}

func (clipboardTarget) Insert(_ context.Context, text string) error {
	return win32.SetClipboardText(text)
}

// copiedInstead hands the prompt to the clipboard when the window it was meant
// for cannot take it. A delivery that lost the race for the focus leaves the
// author with the prompt to paste by hand rather than with nothing at all; the
// words that come back still say what happened, because the window did not get
// the prompt and the author must know that.
type copiedInstead struct {
	primary promptflow.Target
}

func (c copiedInstead) Insert(ctx context.Context, text string) error {
	err := c.primary.Insert(ctx, text)
	if err == nil {
		return nil
	}
	if copyErr := (clipboardTarget{}).Insert(ctx, text); copyErr != nil {
		return err
	}
	return fmt.Errorf("%w; the prompt is on the clipboard instead", err)
}
