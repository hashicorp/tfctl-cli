// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tfe "github.com/hashicorp/go-tfe/v2"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdStatus returns the `auth status` command for displaying auth info.
func NewCmdStatus(inv *cmd.Invocation) *cmd.Command {
	opts := &StatusOpts{
		IO:      inv.IO,
		Profile: inv.Profile,
		Output:  inv.Output,
	}

	cmd := &cmd.Command{
		Name:      "status",
		ShortHelp: "Display information about the current authentication.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s auth status" }} command displays
		information about the currently authenticated account and token,
		including when the token expires if that information is available.
		`, version.Name),
		Examples: []cmd.Example{
			{
				Preamble: "Show authentication status:",
				Command:  fmt.Sprintf("$ %s auth status", version.Name),
			},
		},
		NoAuthRequired: true,
		RunF: func(_ *cmd.Command, _ []string) error {
			if inv.Profile.GetToken() != "" {
				apiClient, err := inv.NewAPIClient()
				if err != nil {
					return fmt.Errorf("failed to create API client: %w", err)
				}
				opts.APIClient = apiClient
			}
			return runStatus(inv.ShutdownCtx, opts)
		},
	}

	return cmd
}

// StatusOpts defines the options for the `auth status` command.
type StatusOpts struct {
	IO        iostreams.IOStreams
	Profile   *profile.Profile
	Output    *format.Outputter
	APIClient *client.Client
}

// StatusResult is the structured output for auth status.
type StatusResult struct {
	Hostname  string     `json:"hostname"`
	Username  string     `json:"username,omitempty"`
	TokenType string     `json:"token_type,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Active    bool       `json:"active"`
	// Reason is a machine-readable cause when Active is false: one of
	// "no_token", "rejected", "server_error", or "unreachable".
	Reason string `json:"reason,omitempty"`
}

func runStatus(ctx context.Context, opts *StatusOpts) error {
	hostname := opts.Profile.GetHostname()

	// No token configured at all.
	if opts.Profile.GetToken() == "" {
		return displayAuthFailure(opts, hostname, &authFailure{reason: reasonNoToken})
	}

	apiClient := opts.APIClient

	// Call /account/details.
	resp, err := apiClient.TFE.API.Account().Details().Get(ctx, nil)
	if err != nil {
		return displayAuthFailure(opts, hostname, classifyAuthError(err))
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
				expiresAt = fetchTokenExpiration(ctx, apiClient, pathStr)
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

// Machine-readable failure reasons surfaced in StatusResult.Reason.
const (
	reasonNoToken     = "no_token"
	reasonRejected    = "rejected"
	reasonServerError = "server_error"
	reasonUnreachable = "unreachable"
)

// authFailure describes why `auth status` could not confirm an active session.
type authFailure struct {
	reason string // one of the reason* constants
	status int    // HTTP status when known (0 otherwise)
	err    error  // underlying transport/server error, when relevant
}

// classifyAuthError turns an /account/details error into an authFailure. A 401
// means the token was rejected — expired or revoked, or (on SAML-SSO-protected
// Terraform Enterprise) a browser SSO session that has lapsed. Any other HTTP
// status is a server-side problem, and an error carrying no HTTP status is a
// connectivity problem; neither of those is an authentication failure, so we
// say so rather than reporting a misleading "unauthorized".
//
// The go-tfe *APIError is wrapped in a *url.Error, so we rely on errors.As to
// walk the chain rather than matching the concrete top-level type.
func classifyAuthError(err error) *authFailure {
	var apiErr *tfe.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusUnauthorized {
			return &authFailure{reason: reasonRejected, status: apiErr.StatusCode}
		}
		return &authFailure{reason: reasonServerError, status: apiErr.StatusCode, err: err}
	}
	return &authFailure{reason: reasonUnreachable, err: err}
}

// displayAuthFailure emits a machine-readable inactive result for JSON/agent
// consumers and writes a cause-specific, actionable message to stderr. It always
// returns cmd.ErrUnderlyingError so callers can tail-call it.
func displayAuthFailure(opts *StatusOpts, hostname string, f *authFailure) error {
	if opts.Output.GetFormat().IsJSONOrAgent() {
		result := &StatusResult{Active: false, Hostname: hostname, Reason: f.reason}
		// Best-effort: ignore display errors since we are already in a failure path.
		_ = opts.Output.Display(&statusDisplayer{result: result, io: opts.IO})
	}

	cs := opts.IO.ColorScheme()
	w := opts.IO.Err()
	icon := cs.FailureIcon()

	switch f.reason {
	case reasonNoToken:
		fmt.Fprintf(w, "%s No token configured for %s. Run '%s auth login' to authenticate.\n",
			icon, hostname, version.Name)
	case reasonRejected:
		fmt.Fprintf(w, "%s Token for %s was rejected (HTTP 401).\n", icon, hostname)
		fmt.Fprintf(w, "  - The token may be expired or revoked: run '%s auth login' to create a new one.\n", version.Name)
		fmt.Fprintf(w, "  - On SSO-protected Terraform Enterprise, your browser SSO session may have lapsed: re-authenticate in the browser, then retry.\n")
	case reasonServerError:
		fmt.Fprintf(w, "%s %s returned HTTP %d (not an authentication problem). Retry, or check the instance status.\n",
			icon, hostname, f.status)
	default: // reasonUnreachable
		if f.err != nil {
			fmt.Fprintf(w, "%s Could not reach %s: %v\n", icon, hostname, f.err)
		} else {
			fmt.Fprintf(w, "%s Could not reach %s.\n", icon, hostname)
		}
	}

	return cmd.ErrUnderlyingError
}

// fetchTokenExpiration follows the auth-token link and returns the expiration
// time if available. Returns nil on any error (best-effort).
func fetchTokenExpiration(ctx context.Context, apiClient *client.Client, path string) *time.Time {
	// The path from the API is already root-absolute (e.g. /api/v2/authentication-tokens/at-xxx),
	// so we replace BaseURL.Path entirely rather than appending to it.
	// Do NOT use client.ResolveURL here: that helper appends to the base path
	// (which already contains /api/v2), which would produce a double-prefixed
	// path like /api/v2/api/v2/authentication-tokens/at-xxx.
	tokenURL := *apiClient.BaseURL
	tokenURL.Path = path
	tokenURL.RawQuery = ""
	tokenURL.Fragment = ""

	resp, err := apiClient.Do(ctx, &client.Request{
		Method: "GET",
		URL:    &tokenURL,
	})
	if err != nil || resp.StatusCode != 200 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil
	}

	var tokenResp struct {
		Data struct {
			Attributes struct {
				ExpiredAt *time.Time `json:"expired-at"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
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
