package tviewui

import (
	"errors"
	"testing"

	core "github.com/example/aws-shell/internal/app"
)

func TestStatusMessageForSelection(t *testing.T) {
	tests := []struct {
		name    string
		result  selectionResponse
		wantMsg string
		wantOK  bool
		wantSpin bool
	}{
		{
			name:    "refresh action",
			result:  selectionResponse{option: optionWithValue(core.WizardActionRefresh)},
			wantMsg: "Refreshing",
			wantOK:  true,
			wantSpin: true,
		},
		{
			name:    "back action",
			result:  selectionResponse{option: optionWithValue(core.WizardActionBack)},
			wantMsg: "Returning to previous step",
			wantOK:  true,
			wantSpin: true,
		},
		{
			name:    "normal selection",
			result:  selectionResponse{option: optionWithValue("cluster-1")},
			wantMsg: "Loading next step",
			wantOK:  true,
			wantSpin: true,
		},
		{
			name:    "error result",
			result:  selectionResponse{err: errors.New("boom")},
			wantMsg: "",
			wantOK:  false,
			wantSpin: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMsg, gotOK, gotSpin := statusMessageForSelection(tc.result)
			if gotMsg != tc.wantMsg || gotOK != tc.wantOK || gotSpin != tc.wantSpin {
				t.Fatalf("got (%q, %t, %t), want (%q, %t, %t)", gotMsg, gotOK, gotSpin, tc.wantMsg, tc.wantOK, tc.wantSpin)
			}
		})
	}
}

func optionWithValue(value string) core.Option {
	return core.Option{Value: value}
}

func TestHasTableHeaderOption(t *testing.T) {
	if hasTableHeaderOption(nil) {
		t.Fatalf("expected false for empty options")
	}
	if hasTableHeaderOption([]core.Option{{Value: "cluster-1"}}) {
		t.Fatalf("expected false for regular option")
	}
	if !hasTableHeaderOption([]core.Option{{Value: core.WizardTableHeaderValue}, {Value: "cluster-1"}}) {
		t.Fatalf("expected true when first option is table header")
	}
}
