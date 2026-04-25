package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/example/aws-shell/internal/app"
	awsecs "github.com/example/aws-shell/internal/aws"
	"github.com/example/aws-shell/internal/prompt"
	"github.com/example/aws-shell/internal/system"
	"github.com/example/aws-shell/internal/tviewui"
)

func main() {
	mode := flag.String("mode", app.ModeBootstrap, "execution mode: bootstrap or aws")
	profile := flag.String("profile", "", "aws-vault profile name")
	flag.Parse()

	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to determine executable path: %v\n", err)
		os.Exit(1)
	}

	runner := system.NewExecRunner(os.Stderr)
	selector := prompt.NewPromptUISelector()
	uiAdapter := tviewui.NewAdapter(os.Stdout, os.Stderr)

	ecsFactory := awsecs.NewSDKFactory()
	prefs := app.NewFileProfilePreferenceStore()

	cli := app.NewCLI(app.Dependencies{
		Runner:      runner,
		Selector:    selector,
		UI:          uiAdapter,
		ECS:         ecsFactory,
		Preferences: prefs,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	})

	code := cli.Run(context.Background(), app.Config{
		Mode:       *mode,
		Profile:    *profile,
		Executable: executable,
	})

	os.Exit(code)
}
