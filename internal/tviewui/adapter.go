package tviewui

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	core "github.com/example/aws-shell/internal/app"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Adapter struct {
	stdout io.Writer
	stderr io.Writer

	mu sync.Mutex

	headerLines  []string
	headerBuffer []string

	app        *tview.Application
	headerView *tview.TextView
	listView   *tview.List
	statusView *tview.TextView

	uiStarted bool

	activeOptions []core.Option
	activeResp    chan selectionResponse
	activeHasTableHeader bool

	statusSpinToken uint64
}

type selectionResponse struct {
	option core.Option
	err    error
}

func NewAdapter(stdout io.Writer, stderr io.Writer) *Adapter {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	return &Adapter{stdout: stdout, stderr: stderr}
}

func (a *Adapter) Select(label string, options []core.Option, defaultIndex int) (core.Option, error) {
	if len(options) == 0 {
		return core.Option{}, fmt.Errorf("no options available")
	}
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}

	if err := a.ensureUIStarted(); err != nil {
		return core.Option{}, err
	}

	resp := make(chan selectionResponse, 1)

	a.mu.Lock()
	a.activeOptions = append([]core.Option{}, options...)
	a.activeResp = resp
	a.activeHasTableHeader = hasTableHeaderOption(options)
	activeHasTableHeader := a.activeHasTableHeader
	headerText := strings.Join(a.headerLines, "\n")
	a.mu.Unlock()

	a.app.QueueUpdateDraw(func() {
		a.headerView.SetText(headerText)
		a.listView.Clear()
		a.listView.SetTitle(" " + label + " ")
		for _, option := range options {
			a.listView.AddItem(option.Label, "", 0, nil)
		}
		currentIndex := defaultIndex
		if activeHasTableHeader && currentIndex == 0 && len(options) > 1 {
			currentIndex = 1
		}
		a.listView.SetCurrentItem(currentIndex)
		a.statusView.SetText("")
	})
	a.stopStatusSpinner()

	result := <-resp
	if result.err != nil {
		return core.Option{}, result.err
	}

	return result.option, nil
}

func (a *Adapter) Confirm(label string, confirmLabel string, cancelLabel string, defaultConfirm bool) (bool, error) {
	defaultIndex := 0
	if !defaultConfirm {
		defaultIndex = 1
	}

	selection, err := a.Select(label, []core.Option{{Label: confirmLabel, Value: "yes"}, {Label: cancelLabel, Value: "no"}}, defaultIndex)
	if err != nil {
		return false, err
	}

	return selection.Value == "yes", nil
}

func (a *Adapter) Message(message string) {
	if a.captureHeaderLine(message) {
		return
	}

	a.mu.Lock()
	app := a.app
	started := a.uiStarted && app != nil
	a.mu.Unlock()
	if started {
		a.setStatusText(message, false)
		return
	}

	fmt.Fprintln(a.stdout, message)
}

func (a *Adapter) Error(message string) {
	a.mu.Lock()
	app := a.app
	started := a.uiStarted && app != nil
	a.mu.Unlock()
	if started {
		a.setStatusText(message, false)
	}

	fmt.Fprintln(a.stderr, message)
}

func (a *Adapter) RetryOrBack(label string, allowBack bool) (core.RetryBackAction, error) {
	options := []core.Option{{Label: "Retry", Value: string(core.RetryBackActionRetry)}}
	if allowBack {
		options = append(options, core.Option{Label: "Back", Value: string(core.RetryBackActionBack)})
	}

	selection, err := a.Select(label, options, 0)
	if err != nil {
		return "", err
	}
	if selection.Value == string(core.RetryBackActionBack) {
		return core.RetryBackActionBack, nil
	}

	return core.RetryBackActionRetry, nil
}

func (a *Adapter) Quit() error {
	a.mu.Lock()
	app := a.app
	started := a.uiStarted && app != nil
	a.statusSpinToken++
	a.uiStarted = false
	a.mu.Unlock()

	if started {
		app.Stop()
	}

	return nil
}

func (a *Adapter) captureHeaderLine(message string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	trimmed := strings.TrimSpace(message)
	if trimmed == "=== ECS Connect Wizard ===" {
		a.headerBuffer = []string{message}
		return true
	}

	if len(a.headerBuffer) > 0 {
		a.headerBuffer = append(a.headerBuffer, message)
		if len(a.headerBuffer) >= 5 {
			a.headerLines = append([]string{}, a.headerBuffer[:5]...)
			a.headerBuffer = nil
		}
		return true
	}

	return false
}

func (a *Adapter) ensureUIStarted() error {
	a.mu.Lock()
	if a.uiStarted {
		a.mu.Unlock()
		return nil
	}

	a.app = tview.NewApplication()
	a.headerView = tview.NewTextView().SetDynamicColors(false).SetWrap(false)
	a.headerView.SetBorder(false)
	a.listView = tview.NewList().ShowSecondaryText(false)
	a.listView.SetBorder(true)
	a.statusView = tview.NewTextView().SetDynamicColors(false).SetWrap(true)
	a.statusView.SetBorder(false)

	a.listView.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		a.completeSelectionAtIndex(index)
	})
	a.listView.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if index != 0 || !a.hasActiveTableHeader() {
			return
		}

		a.mu.Lock()
		optionsCount := len(a.activeOptions)
		a.mu.Unlock()
		if optionsCount <= 1 {
			return
		}

		a.listView.SetCurrentItem(1)
	})
	a.listView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlC:
			a.completeSelection(selectionResponse{err: core.ErrSelectionCanceled})
			return nil
		case tcell.KeyUp:
			if a.hasActiveTableHeader() && a.listView.GetCurrentItem() <= 1 {
				a.listView.SetCurrentItem(1)
				return nil
			}
		case tcell.KeyHome:
			if a.hasActiveTableHeader() {
				a.listView.SetCurrentItem(1)
				return nil
			}
		}

		switch event.Rune() {
		case 'q', 'Q':
			a.completeSelection(selectionResponse{option: core.Option{Label: "Quit", Value: core.WizardActionQuit}})
			return nil
		case 'b', 'B':
			a.completeSelection(selectionResponse{option: core.Option{Label: "Back", Value: core.WizardActionBack}})
			return nil
		case 'r', 'R':
			a.completeSelection(selectionResponse{option: core.Option{Label: "Refresh", Value: core.WizardActionRefresh}})
			return nil
		}

		return event
	})

	root := tview.NewFlex().SetDirection(tview.FlexRow)
	root.AddItem(a.headerView, 5, 0, false)
	root.AddItem(a.listView, 0, 1, true)
	root.AddItem(a.statusView, 1, 0, false)

	a.uiStarted = true
	a.mu.Unlock()

	go func() {
		_ = a.app.SetRoot(root, true).Run()
	}()

	return nil
}

func hasTableHeaderOption(options []core.Option) bool {
	if len(options) == 0 {
		return false
	}

	return options[0].Value == core.WizardTableHeaderValue
}

func (a *Adapter) hasActiveTableHeader() bool {
	a.mu.Lock()
	active := a.activeHasTableHeader
	a.mu.Unlock()

	return active
}

func (a *Adapter) completeSelectionAtIndex(index int) {
	a.mu.Lock()
	if index < 0 || index >= len(a.activeOptions) {
		a.mu.Unlock()
		return
	}
	option := a.activeOptions[index]
	a.mu.Unlock()

	a.completeSelection(selectionResponse{option: option})
}

func (a *Adapter) completeSelection(result selectionResponse) {
	a.mu.Lock()
	resp := a.activeResp
	a.activeResp = nil
	a.activeOptions = nil
	a.activeHasTableHeader = false
	a.mu.Unlock()

	if resp == nil {
		return
	}

	select {
	case resp <- result:
	default:
	}

	a.mu.Lock()
	app := a.app
	started := a.uiStarted && app != nil
	a.mu.Unlock()

	if started {
		if errors.Is(result.err, core.ErrSelectionCanceled) || result.option.Value == core.WizardActionQuit {
			go func() {
				_ = a.Quit()
			}()
			return
		}

		statusMessage, shouldShow, shouldSpin := statusMessageForSelection(result)
		if !shouldShow {
			return
		}

		a.setStatusText(statusMessage, shouldSpin)
	}
}

func statusMessageForSelection(result selectionResponse) (string, bool, bool) {
	if result.err != nil {
		return "", false, false
	}

	switch result.option.Value {
	case core.WizardActionRefresh:
		return "Refreshing", true, true
	case core.WizardActionBack:
		return "Returning to previous step", true, true
	default:
		return "Loading next step", true, true
	}
}

func (a *Adapter) setStatusText(message string, spinning bool) {
	a.mu.Lock()
	app := a.app
	view := a.statusView
	started := a.uiStarted && app != nil && view != nil
	if !started {
		a.statusSpinToken++
		a.mu.Unlock()
		return
	}

	a.statusSpinToken++
	token := a.statusSpinToken
	a.mu.Unlock()

	if !spinning {
		go app.QueueUpdateDraw(func() {
			view.SetText(message)
		})
		return
	}

	go a.runStatusSpinner(token, message)
}

func (a *Adapter) runStatusSpinner(token uint64, message string) {
	frames := []string{"|", "/", "-", "\\"}
	frameIndex := 0
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	for {
		a.mu.Lock()
		app := a.app
		view := a.statusView
		active := a.uiStarted && app != nil && view != nil && token == a.statusSpinToken
		a.mu.Unlock()

		if !active {
			return
		}

		text := fmt.Sprintf("%s %s", message, frames[frameIndex])
		go app.QueueUpdateDraw(func() {
			view.SetText(text)
		})
		frameIndex = (frameIndex + 1) % len(frames)

		<-ticker.C
	}
}

func (a *Adapter) stopStatusSpinner() {
	a.mu.Lock()
	a.statusSpinToken++
	a.mu.Unlock()
}

func (a *Adapter) Suspend(run func()) {
	if run == nil {
		return
	}

	a.mu.Lock()
	app := a.app
	started := a.uiStarted && app != nil
	a.mu.Unlock()

	if !started {
		run()
		return
	}

	app.Suspend(run)
}
