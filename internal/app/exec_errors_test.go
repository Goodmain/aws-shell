package app

import (
	"strings"
	"testing"
)

func TestFormatConnectErrorMappings(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want string
	}{
		{name: "exec disabled", err: "The execute command failed because execute command was not enabled", want: "ECS Exec is not enabled"},
		{name: "access denied", err: "AccessDeniedException: User is not authorized to perform: ssm:StartSession", want: "Access denied"},
		{name: "missing plugin", err: "session-manager-plugin was not found", want: "session-manager-plugin"},
		{name: "unknown", err: "some other error", want: "some other error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatConnectError("dev", errString(tt.err))
			if got == "" || !strings.Contains(got, tt.want) {
				t.Fatalf("got %q, expected to contain %q", got, tt.want)
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }
