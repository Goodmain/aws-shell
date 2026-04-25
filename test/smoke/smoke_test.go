//go:build integration

package smoke

import (
	"os"
	"os/exec"
	"testing"
)

func TestInteractiveFlowSmoke(t *testing.T) {
	profile := os.Getenv("AWS_VAULT_SMOKE_PROFILE")
	if profile == "" {
		t.Skip("set AWS_VAULT_SMOKE_PROFILE to run smoke test")
	}

	cmd := exec.Command("go", "run", "../../cmd/aws-shell", "--mode", "aws", "--profile", profile)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("smoke test failed: %v\n%s", err, string(out))
	}
}
