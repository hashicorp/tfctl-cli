// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

// NewCmdStatus returns the `auth status` command for displaying auth info.
func NewCmdStatus(ctx *cmd.Context) *cmd.Command {
	opts := &StatusOpts{
		Ctx:     ctx.ShutdownCtx,
		IO:      ctx.IO,
		Profile: ctx.Profile,
		Output:  ctx.Output,
	}

	cmd := &cmd.Command{
		Name:      "status",
		ShortHelp: "Display information about the current authentication.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s auth status" }} command displays
		information about the currently authenticated account and token,
		including when the token expires if that information is available.
		`, config.Name),
		Examples: []cmd.Example{
			{
				Preamble: "Show authentication status:",
				Command:  fmt.Sprintf("$ %s auth status", config.Name),
			},
		},
		NoAuthRequired: true,
		RunF: func(_ *cmd.Command, _ []string) error {
			return runStatus(opts)
		},
	}

	return cmd
}

// StatusOpts defines the options for the `auth status` command.
type StatusOpts struct {
	Ctx     context.Context
	IO      iostreams.IOStreams
	Profile *profile.Profile
	Output  *format.Outputter
}

// StatusResult is the structured output for auth status.
type StatusResult struct {
	Hostname  string     `json:"hostname"`
	Username  string     `json:"username"`
	TokenType string     `json:"token_type"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Active    bool       `json:"active"`
}

func runStatus(opts *StatusOpts) error {
	hostname := opts.Profile.Hostname
	if hostname == "" {
		hostname = defaultHostname
	}

	cs := opts.IO.ColorScheme()

	// No token configured at all.
	if opts.Profile.Token == "" {
		fmt.Fprintf(opts.IO.Err(), "%s Unauthorized for %s\n", cs.FailureIcon(), hostname)
		return cmd.ErrUnderlyingError
	}

	// Build a one-off API client from the profile.
	address := opts.Profile.Hostname
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "https://" + address
	}
	apiClient, err := client.New(address, opts.Profile.Token, nil)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Call /account/details.
	resp, err := apiClient.TFE.API.Account().Details().Get(opts.Ctx, nil)
	if err != nil {
		fmt.Fprintf(opts.IO.Err(), "%s Unauthorized for %s\n", cs.FailureIcon(), hostname)

		var apiErr *tfe.APIError
		if errors.As(err, &apiErr) {
			return cmd.ErrUnderlyingError
		}
		return cmd.ErrUnderlyingError
	}

	data := resp.GetData()
	if data == nil {
		return fmt.Errorf("empty response from account details")
	}

	attrs := data.GetAttributes()
	if attrs == nil || attrs.GetUsername() == nil {
		return fmt.Errorf("no username in account details response")
	}

	username := *attrs.GetUsername()

	// Determine token type from the authenticated-resource relationship.
	tokenType := "user"
	rels := data.GetRelationships()
	if rels != nil {
		if authRes := rels.GetAuthenticatedResource(); authRes != nil {
			if authResData := authRes.GetData(); authResData != nil {
				if t := authResData.GetTypeEscaped(); t != nil {
					tokenType = t.String()
				}
			}
		}
	}

	// Follow the auth-token link for expiration information.
	var expiresAt *time.Time
	if links := data.GetLinks(); links != nil {
		if authTokenPath, ok := links.GetAdditionalData()["auth-token"]; ok {
			var pathStr string
			switch v := authTokenPath.(type) {
			case string:
				pathStr = v
			case *string:
				if v != nil {
					pathStr = *v
				}
			}
			if pathStr != "" {
				expiresAt = fetchTokenExpiration(opts.Ctx, apiClient, pathStr)
			}
		}
	}

	result := &StatusResult{
		Hostname:  hostname,
		Username:  username,
		TokenType: tokenType,
		ExpiresAt: expiresAt,
		Active:    true,
	}

	return opts.Output.Display(&statusDisplayer{result: result, io: opts.IO})
}

// fetchTokenExpiration follows the auth-token link and returns the expiration
// time if available. Returns nil on any error (best-effort).
func fetchTokenExpiration(ctx context.Context, apiClient *client.Client, path string) *time.Time {
	// The path from the API is absolute (e.g., /api/v2/authentication-tokens/at-xxx).
	// Build the URL from the base URL's scheme+host, not using the full API base path.
	tokenURL := *apiClient.BaseURL
	tokenURL.Path = path
	tokenURL.RawQuery = ""
	tokenURL.Fragment = ""

	resp, err := apiClient.RawRequest(ctx, &client.Request{
		Method: "GET",
		URL:    &tokenURL,
	})
	if err != nil || resp.StatusCode != 200 {
		return nil
	}

	var tokenResp struct {
		Data struct {
			Attributes struct {
				ExpiredAt *time.Time `json:"expired-at"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &tokenResp); err != nil {
		return nil
	}

	return tokenResp.Data.Attributes.ExpiredAt
}

// statusDisplayer implements format.Displayer and format.StringPayload.
type statusDisplayer struct {
	result *StatusResult
	io     iostreams.IOStreams
}

var (
	_ format.Displayer     = (*statusDisplayer)(nil)
	_ format.StringPayload = (*statusDisplayer)(nil)
)

func (d *statusDisplayer) DefaultFormat() format.Format { return format.Pretty }
func (d *statusDisplayer) Payload() any                 { return d.result }
func (d *statusDisplayer) FieldTemplates() []format.Field {
	return nil
}

// StringPayload returns pre-formatted output for pretty and markdown formats.
func (d *statusDisplayer) StringPayload(f format.Format) string {
	var sb strings.Builder
	cs := d.io.ColorScheme()

	switch f {
	case format.Markdown:
		fmt.Fprintf(&sb, "Logged in to %s (%s: %s)", d.result.Hostname, d.result.TokenType, d.result.Username)
		if d.result.ExpiresAt != nil {
			fmt.Fprintf(&sb, "\nToken expires: %s", d.result.ExpiresAt.Format(time.RFC3339))
		}
	default:
		fmt.Fprintf(&sb, "%s Logged in to %s (%s: %s)",
			cs.SuccessIcon(),
			d.result.Hostname,
			d.result.TokenType,
			cs.String(d.result.Username).Bold())
		if d.result.ExpiresAt != nil {
			fmt.Fprintf(&sb, "\n%s Token expires: %s",
				cs.SuccessIcon(),
				d.result.ExpiresAt.Format(time.RFC3339))
		}
	}

	return sb.String()
}
