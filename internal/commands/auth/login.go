// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	tfe "github.com/hashicorp/go-tfe/v2"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

const (
	// tokenPagePath is the path to the token creation page.
	tokenPagePath = "/app/settings/tokens?source=" + version.Name + "-login"
)

// errLoginCanceled signals that the user declined the interactive confirmation
// prompt shown before the browser is opened. loginRun treats it as a clean,
// non-error exit rather than a failure.
var errLoginCanceled = errors.New("login canceled")

// NewCmdLogin returns the `auth login` command for authenticating.
func NewCmdLogin(inv *cmd.Invocation) *cmd.Command {
	opts := &LoginOpts{
		IO:          inv.IO,
		Profile:     inv.Profile,
		Output:      inv.Output,
		OpenBrowser: openBrowser,
	}

	cmd := &cmd.Command{
		Name:      "login",
		ShortHelp: "Authenticate with HCP Terraform or Terraform Enterprise.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%[1]s auth login" }} command authenticates
		the %[1]s CLI with HCP Terraform or Terraform Enterprise.

		When {{ template "mdCodeOrBold" "--token" }} is specified, the token is read
		from standard input. This is useful for non-interactive environments such as
		CI/CD pipelines.

		When {{ template "mdCodeOrBold" "--token" }} is not specified, the browser is
		opened to the token creation page for the configured hostname and the user is
		prompted to paste the generated token.
		`, version.Name),
		Examples: []cmd.Example{
			{
				Preamble: "Login interactively:",
				Command:  fmt.Sprintf("$ %s auth login", version.Name),
			},
			{
				Preamble: "Login with a token from stdin:",
				Command:  fmt.Sprintf("$ echo my-token | %s auth login --token", version.Name),
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
			opts.DryRun = inv.IsDryRun()
			return loginRun(inv.ShutdownCtx, inv, opts)
		},
	}

	return cmd
}

// LoginOpts defines the options for the `auth login` command.
type LoginOpts struct {
	IO      iostreams.IOStreams
	Profile *profile.Profile
	Output  *format.Outputter

	// OpenBrowser opens a URL in the user's default browser. When nil, the
	// package default openBrowser is used. Tests inject a no-op opener to avoid
	// launching a real browser and to keep parallel tests free of shared state.
	OpenBrowser func(url string) error

	Name   string
	Token  bool
	DryRun bool
}

func loginRun(ctx context.Context, inv *cmd.Invocation, opts *LoginOpts) error {
	hostname := opts.Profile.GetHostname()
	logger := logging.FromContext(ctx)

	logger.Debug("starting login process", "hostname", hostname, "token_from_stdin", opts.Token)

	// Read the token.
	var token string
	var err error
	if opts.Token {
		token, err = readTokenFromStdin(opts)
	} else {
		token, err = readTokenInteractive(opts, hostname)
	}
	if err != nil {
		// A declined confirmation is a clean, user-initiated exit, not a failure.
		if errors.Is(err, errLoginCanceled) {
			return nil
		}
		return err
	}

	// Set the token on the profile and create a client to verify it.
	opts.Profile.Token = token
	logger.Debug("verifying token", "hostname", hostname)
	apiClient, err := inv.NewAPIClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	return saveToken(ctx, opts, apiClient, hostname, token)
}

// readTokenFromStdin reads a token from stdin.
func readTokenFromStdin(opts *LoginOpts) (string, error) {
	scanner := bufio.NewScanner(opts.IO.In())
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("failed to read token from stdin: %w", err)
		}
		return "", fmt.Errorf("no token provided on stdin")
	}

	token := strings.TrimSpace(scanner.Text())
	if token == "" {
		return "", fmt.Errorf("token is empty")
	}

	return token, nil
}

// readTokenInteractive explains the flow, asks the user to confirm, opens the
// browser to the token page, and prompts for the generated token. The user must
// confirm before the browser is opened; declining is a clean exit.
func readTokenInteractive(opts *LoginOpts, hostname string) (string, error) {
	if !opts.IO.CanPrompt() {
		return "", fmt.Errorf("interactive login requires a terminal; use --token to read from stdin")
	}

	tokenURL := fmt.Sprintf("https://%s%s", hostname, tokenPagePath)
	cs := opts.IO.ColorScheme()

	// Explain what is about to happen and confirm before opening the browser.
	fmt.Fprintf(opts.IO.Err(),
		"%s will open the following URL in your browser to create an API token for %s:\n\n  %s\n\n",
		version.Name,
		cs.String(hostname).Bold().String(),
		cs.String(tokenURL).Bold().String(),
	)

	proceed, err := opts.IO.PromptConfirm("Do you want to proceed")
	if err != nil {
		return "", fmt.Errorf("failed to read confirmation: %w", err)
	}
	if !proceed {
		fmt.Fprintln(opts.IO.Err(), "Login canceled.")
		return "", errLoginCanceled
	}

	// Open the browser to the token creation page.
	fmt.Fprint(opts.IO.Err(), "\nOpening your browser to create a token...\n\n")

	openURL := opts.OpenBrowser
	if openURL == nil {
		openURL = openBrowser
	}
	if err := openURL(tokenURL); err != nil {
		fmt.Fprintf(opts.IO.Err(),
			"%s Could not open the browser automatically. Open the URL above manually.\n\n",
			cs.WarningLabel())
	}

	// Prompt for the token and read it without echoing.
	fmt.Fprintf(opts.IO.Err(),
		"After creating the token, copy it and paste it below.\n%s will store it for %s.\n\nPaste your token: ",
		version.Name, cs.String(hostname).Bold().String())

	secret, err := opts.IO.ReadSecret()
	if err != nil {
		return "", fmt.Errorf("failed to read token: %w", err)
	}
	fmt.Fprintln(opts.IO.Err())

	token := strings.TrimSpace(string(secret))
	if token == "" {
		return "", fmt.Errorf("token is empty")
	}

	return token, nil
}

// saveToken verifies the token via the API and persists it to the profile.
func saveToken(ctx context.Context, opts *LoginOpts, apiClient *client.Client, hostname, token string) error {
	cs := opts.IO.ColorScheme()

	user, err := verifyToken(ctx, apiClient)
	if err != nil {
		return fmt.Errorf("failed to verify token: %w", err)
	}

	if opts.DryRun {
		fmt.Fprintf(opts.IO.Err(), "%s would save token to profile %q for host %s (user: %s)\n",
			cs.DryRunLabel(), opts.Profile.Name, hostname, user)
		return nil
	}

	opts.Profile.Token = token
	if err := opts.Profile.Write(); err != nil {
		return fmt.Errorf("failed to save token to profile: %w", err)
	}

	fmt.Fprintf(opts.IO.Err(), "%s Successfully logged in to %s as %s\n",
		cs.SuccessIcon(), hostname, cs.String(user).Bold().String())
	return nil
}

// verifyToken makes an API call to validate the token and returns the username.
func verifyToken(ctx context.Context, apiClient *client.Client) (string, error) {
	resp, err := apiClient.TFE.API.Account().Details().Get(ctx, nil)
	if err != nil {
		var apiErr *tfe.APIError
		if errors.As(err, &apiErr) {
			return "", fmt.Errorf("token validation failed: %s", apiErr.Error())
		}
		return "", fmt.Errorf("token validation failed: %w", err)
	}

	data := resp.GetData()
	if data == nil {
		return "", fmt.Errorf("token validation failed: empty response")
	}

	attrs := data.GetAttributes()
	if attrs == nil || attrs.GetUsername() == nil {
		return "", fmt.Errorf("token validation failed: no username in response")
	}

	return *attrs.GetUsername(), nil
}
