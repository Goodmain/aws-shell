package prompt

import (
	"errors"

	"github.com/example/aws-shell/internal/app"
	"github.com/manifoldco/promptui"
)

type PromptUISelector struct{}

func NewPromptUISelector() *PromptUISelector {
	return &PromptUISelector{}
}

func (s *PromptUISelector) Select(label string, options []app.Option, defaultIndex int) (app.Option, error) {
	if len(options) == 0 {
		return app.Option{}, errors.New("no options available")
	}
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}

	labels := make([]string, 0, len(options))
	for _, option := range options {
		labels = append(labels, option.Label)
	}

	p := promptui.Select{
		Label:     label,
		Items:     labels,
		Size:      15,
		CursorPos: defaultIndex,
	}

	idx, _, err := p.Run()
	if err != nil {
		if err == promptui.ErrInterrupt || err == promptui.ErrEOF {
			return app.Option{}, app.ErrSelectionCanceled
		}

		return app.Option{}, err
	}

	return options[idx], nil
}
