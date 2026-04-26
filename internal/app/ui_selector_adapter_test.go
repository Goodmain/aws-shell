package app

import (
	"errors"
	"strings"
	"testing"
)

type selectorStub struct {
	selection Option
	err       error

	lastLabel        string
	lastOptions      []Option
	lastDefaultIndex int
}

func (s *selectorStub) Select(label string, options []Option, defaultIndex int) (Option, error) {
	s.lastLabel = label
	s.lastOptions = append([]Option{}, options...)
	s.lastDefaultIndex = defaultIndex

	if s.err != nil {
		return Option{}, s.err
	}
	if s.selection.Value != "" {
		return s.selection, nil
	}

	return options[defaultIndex], nil
}

func TestSelectorUIAdapterSelectRequiresSelector(t *testing.T) {
	adapter := NewSelectorUIAdapter(nil, nil, nil)
	_, err := adapter.Select("pick", []Option{{Label: "a", Value: "a"}}, 0)
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "selector is not configured" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelectorUIAdapterConfirmDefaultSelectionAndResult(t *testing.T) {
	stub := &selectorStub{selection: Option{Label: "Cancel", Value: "no"}}
	adapter := NewSelectorUIAdapter(stub, nil, nil)

	confirmed, err := adapter.Confirm("Continue?", "Yes", "No", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if confirmed {
		t.Fatalf("expected false confirmation")
	}
	if stub.lastDefaultIndex != 1 {
		t.Fatalf("expected default index 1, got %d", stub.lastDefaultIndex)
	}
}

func TestSelectorUIAdapterConfirmPropagatesError(t *testing.T) {
	stub := &selectorStub{err: errors.New("boom")}
	adapter := NewSelectorUIAdapter(stub, nil, nil)

	_, err := adapter.Confirm("Continue?", "Yes", "No", true)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected propagated error, got %v", err)
	}
}

func TestSelectorUIAdapterMessageAndErrorWriteOutput(t *testing.T) {
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	adapter := NewSelectorUIAdapter(&selectorStub{}, stdout, stderr)

	adapter.Message("hello")
	adapter.Error("oops")

	if stdout.String() != "hello\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "oops\n" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestSelectorUIAdapterRetryOrBack(t *testing.T) {
	stub := &selectorStub{selection: Option{Label: "Back", Value: string(RetryBackActionBack)}}
	adapter := NewSelectorUIAdapter(stub, nil, nil)

	action, err := adapter.RetryOrBack("Try again?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != RetryBackActionBack {
		t.Fatalf("expected back action, got %q", action)
	}
}

func TestSelectorUIAdapterQuitReturnsSelectionCanceled(t *testing.T) {
	adapter := NewSelectorUIAdapter(&selectorStub{}, nil, nil)
	err := adapter.Quit()
	if !errors.Is(err, ErrSelectionCanceled) {
		t.Fatalf("expected ErrSelectionCanceled, got %v", err)
	}
}
