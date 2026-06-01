// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

const (
	// tokenPagePath is the path to the token creation page.
	tokenPagePath = "/app/settings/tokens?source=" + version.Name + "-login"
)

// NewCmdLogin returns the `auth login` command for authenticating.
func NewCmdLogin(ctx *cmd.Context) *cmd.Command {
	opts := &LoginOpts{
		Ctx:     ctx.ShutdownCtx,
		IO:      ctx.IO,
		Profile: ctx.Profile,
		Output:  ctx.Output,
	}

	cmd := &cmd.Command{
		Name:      "login",
		ShortHelp: "Authenticate with HCP Terraform or Terraform Enterprise.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
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
		RunF: func(c *cmd.Command, _ []string) error {
			opts.Logger = c.Logger(ctx)
			opts.DryRun = ctx.IsDryRun()
			return loginRun(ctx, opts)
		},
	}

	return cmd
}

// LoginOpts defines the options for the `auth login` command.
type LoginOpts struct {
	Ctx     context.Context
	IO      iostreams.IOStreams
	Profile *profile.Profile
	Output  *format.Outputter
	Logger  hclog.Logger

	Name   string
	Token  bool
	DryRun bool
}

func loginRun(cmdCtx *cmd.Context, opts *LoginOpts) error {
	hostname := opts.Profile.GetHostname()

	opts.Logger.Debug("starting login process", "hostname", hostname, "token_from_stdin", opts.Token)

	// Read the token.
	var token string
	var err error
	if opts.Token {
		token, err = readTokenFromStdin(opts)
	} else {
		token, err = readTokenInteractive(opts, hostname)
	}
	if err != nil {
		return err
	}

	// Set the token on the profile and create a client to verify it.
	opts.Profile.Token = token
	opts.Logger.Debug("verifying token", "hostname", hostname)
	apiClient, err := cmdCtx.NewAPIClient(opts.Logger)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	return saveToken(opts, apiClient, hostname, token)
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

// readTokenInteractive opens the browser to the token page and prompts the user.
func readTokenInteractive(opts *LoginOpts, hostname string) (string, error) {
	if !opts.IO.CanPrompt() {
		return "", fmt.Errorf("interactive login requires a terminal; use --token to read from stdin")
	}

	tokenURL := fmt.Sprintf("https://%s%s", hostname, tokenPagePath)
	cs := opts.IO.ColorScheme()

	fmt.Fprintf(opts.IO.Err(), "Opening browser to create a token at:\n  %s\n\n",
		cs.String(tokenURL).Bold().String())

	if err := openBrowser(tokenURL); err != nil {
		fmt.Fprintf(opts.IO.Err(), "%s Could not open browser. Please open the URL above manually.\n\n",
			cs.WarningLabel())
	}

	fmt.Fprint(opts.IO.Err(), "Paste your token: ")
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
func saveToken(opts *LoginOpts, apiClient *client.Client, hostname, token string) error {
	cs := opts.IO.ColorScheme()

	user, err := verifyToken(opts.Ctx, apiClient)
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
