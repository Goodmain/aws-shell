package app

import (
	"errors"
	"fmt"
	"io"
)

type SelectorUIAdapter struct {
	selector Selector
	stdout   io.Writer
	stderr   io.Writer
}

func NewSelectorUIAdapter(selector Selector, stdout io.Writer, stderr io.Writer) *SelectorUIAdapter {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	return &SelectorUIAdapter{selector: selector, stdout: stdout, stderr: stderr}
}

func (s *SelectorUIAdapter) Select(label string, options []Option, defaultIndex int) (Option, error) {
	if s.selector == nil {
		return Option{}, errors.New("selector is not configured")
	}

	return s.selector.Select(label, options, defaultIndex)
}

func (s *SelectorUIAdapter) Confirm(label string, confirmLabel string, cancelLabel string, defaultConfirm bool) (bool, error) {
	defaultIndex := 0
	if !defaultConfirm {
		defaultIndex = 1
	}

	selection, err := s.Select(label, []Option{{Label: confirmLabel, Value: "yes"}, {Label: cancelLabel, Value: "no"}}, defaultIndex)
	if err != nil {
		return false, err
	}

	return selection.Value == "yes", nil
}

func (s *SelectorUIAdapter) Message(message string) {
	fmt.Fprintln(s.stdout, message)
}

func (s *SelectorUIAdapter) Error(message string) {
	fmt.Fprintln(s.stderr, message)
}

func (s *SelectorUIAdapter) RetryOrBack(label string, allowBack bool) (RetryBackAction, error) {
	options := []Option{{Label: "Retry", Value: string(RetryBackActionRetry)}}
	if allowBack {
		options = append(options, Option{Label: "Back", Value: string(RetryBackActionBack)})
	}

	selection, err := s.Select(label, options, 0)
	if err != nil {
		return "", err
	}

	if selection.Value == string(RetryBackActionBack) {
		return RetryBackActionBack, nil
	}

	return RetryBackActionRetry, nil
}

func (s *SelectorUIAdapter) Quit() error {
	return ErrSelectionCanceled
}
