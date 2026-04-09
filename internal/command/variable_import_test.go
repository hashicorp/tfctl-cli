package command

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	cli "github.com/hashicorp/cli"

	"github.com/hashicorp/tfcloud/internal/config"
)

func TestVariableImportUsesDefaultOrganizationFromConfig(t *testing.T) {
	t.Parallel()

	ui := cli.NewMockUi()
	cmd := &VariableImportCommand{
		Meta: &Meta{
			UI:          ui,
			Stdin:       bytes.NewBuffer(nil),
			Stdout:      ui.OutputWriter,
			Stderr:      ui.ErrorWriter,
			StdoutIsTTY: false,
			StderrIsTTY: false,
			HumanOutput: true,
		},
		loadConfig: func() (*config.Config, error) {
			return &config.Config{
				Hostname:            "app.terraform.test",
				Token:               "token",
				DefaultOrganization: "config-org",
				DefaultHeaders:      make(http.Header),
			}, nil
		},
	}

	code := cmd.Run([]string{"-e", "AWS_REGION", "-variable-set-name", "production"})
	if code != 1 {
		t.Fatalf("got exit code %d", code)
	}
	if got := ui.ErrorWriter.String(); strings.Contains(got, "-organization is required") {
		t.Fatalf("expected config default organization to satisfy validation, got %q", got)
	}
}
