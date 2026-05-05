// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/flagvalue"
	"github.com/hashicorp/tfcloud/internal/pkg/format"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
)

const (
	// defaultHostname is the default HCP Terraform hostname.
	defaultHostname = "app.terraform.io"

	// tokenPagePath is the path to the token creation page.
	tokenPagePath = "/app/settings/tokens"
)

// NewCmdLogin returns the `tfcloud auth login` command for authenticating.
func NewCmdLogin(ctx *cmd.Context) *cmd.Command {
	opts := &LoginOpts{
		IO:      ctx.IO,
		Profile: ctx.Profile,
		Output:  ctx.Output,
	}

	cmd := &cmd.Command{
		Name:      "login",
		ShortHelp: "Authenticate with HCP Terraform or Terraform Enterprise.",
		LongHelp: heredoc.New(ctx.IO).Must(`
		The {{ template "mdCodeOrBold" "tfcloud auth login" }} command authenticates
		the tfcloud CLI with HCP Terraform or Terraform Enterprise.

		When {{ template "mdCodeOrBold" "--token" }} is specified, the token is read
		from standard input. This is useful for non-interactive environments such as
		CI/CD pipelines.

		When {{ template "mdCodeOrBold" "--token" }} is not specified, the browser is
		opened to the token creation page for the configured hostname and the user is
		prompted to paste the generated token.
		`),
		Examples: []cmd.Example{
			{
				Preamble: "Login interactively:",
				Command:  "$ tfcloud auth login",
			},
			{
				Preamble: "Login with a token from stdin:",
				Command:  "$ echo my-token | tfcloud auth login --token",
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:          "token",
					Description:   "Read the token from standard input instead of prompting.",
					Value:         flagvalue.Simple(false, &opts.Token),
					IsBooleanFlag: true,
				},
			},
		},
		NoAuthRequired: true,
		RunF: func(_ *cmd.Command, _ []string) error {
			opts.Ctx = ctx.ShutdownCtx
			return loginRun(opts)
		},
	}

	return cmd
}

// LoginOpts defines the options for the `tfcloud auth login` command.
type LoginOpts struct {
	Ctx     context.Context
	IO      iostreams.IOStreams
	Profile *profile.Profile
	Output  *format.Outputter

	Token bool
}

func loginRun(opts *LoginOpts) error {
	hostname := opts.Profile.Hostname
	if hostname == "" {
		hostname = defaultHostname
	}

	if opts.Token {
		return loginFromStdin(opts, hostname)
	}

	return loginInteractive(opts, hostname)
}

// loginFromStdin reads a token from stdin and stores it in the profile.
func loginFromStdin(opts *LoginOpts, hostname string) error {
	scanner := bufio.NewScanner(opts.IO.In())
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to read token from stdin: %w", err)
		}
		return fmt.Errorf("no token provided on stdin")
	}

	token := strings.TrimSpace(scanner.Text())
	if token == "" {
		return fmt.Errorf("token is empty")
	}

	opts.Profile.Token = token
	if err := opts.Profile.Write(); err != nil {
		return fmt.Errorf("failed to save token to profile: %w", err)
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.Err(), "%s Successfully logged in to %s\n", cs.SuccessIcon(), hostname)
	return nil
}

// loginInteractive opens the browser to the token page and prompts the user.
func loginInteractive(opts *LoginOpts, hostname string) error {
	if !opts.IO.CanPrompt() {
		return fmt.Errorf("interactive login requires a terminal; use --token to read from stdin")
	}

	tokenURL := fmt.Sprintf("https://%s%s", hostname, tokenPagePath)
	cs := opts.IO.ColorScheme()

	fmt.Fprintf(opts.IO.Err(), "Opening browser to create a token at:\n  %s\n\n",
		cs.String(tokenURL).Bold().String())

	// Attempt to open the browser
	if err := openBrowser(tokenURL); err != nil {
		fmt.Fprintf(opts.IO.Err(), "%s Could not open browser. Please open the URL above manually.\n\n",
			cs.WarningLabel())
	}

	fmt.Fprint(opts.IO.Err(), "Paste your token: ")
	secret, err := opts.IO.ReadSecret()
	if err != nil {
		return fmt.Errorf("failed to read token: %w", err)
	}
	fmt.Fprintln(opts.IO.Err())

	token := strings.TrimSpace(string(secret))
	if token == "" {
		return fmt.Errorf("token is empty")
	}

	opts.Profile.Token = token
	if err := opts.Profile.Write(); err != nil {
		return fmt.Errorf("failed to save token to profile: %w", err)
	}

	fmt.Fprintf(opts.IO.Err(), "\n%s Successfully logged in to %s\n", cs.SuccessIcon(), hostname)
	return nil
}
