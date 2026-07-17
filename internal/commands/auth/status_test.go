// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	tfe "github.com/hashicorp/go-tfe/v2"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

// newFakeStatusTFE returns an httptest.Server that serves /account/details and
// optionally /authentication-tokens/{id}. If username is empty, account/details
// returns 401. If tokenID is non-empty, the auth-token link is included and the
// token endpoint is registered. If expiredAt is non-empty, it is included in
// the token attributes.
func newFakeStatusTFE(t *testing.T, username, tokenType, tokenID, expiredAt string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v2/account/details", func(w http.ResponseWriter, r *http.Request) {
		if username == "" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"errors":[{"status":"401","title":"unauthorized"}]}`)
			return
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")

		authTokenLink := ""
		if tokenID != "" {
			authTokenLink = fmt.Sprintf(`,"auth-token":"/api/v2/authentication-tokens/%s"`, tokenID)
		}

		authResType := "users"
		if tokenType != "" {
			authResType = tokenType
		}

		fmt.Fprintf(w, `{
			"data":{
				"id":"user-abc",
				"type":"users",
				"attributes":{"username":%q},
				"links":{"self":"/api/v2/users/user-abc"%s},
				"relationships":{
					"authenticated-resource":{
						"data":{"id":"user-abc","type":%q},
						"links":{"related":"/api/v2/users/user-abc"}
					}
				}
			}
		}`, username, authTokenLink, authResType)
	})

	if tokenID != "" {
		mux.HandleFunc("/api/v2/authentication-tokens/"+tokenID, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.api+json")

			expiredField := `"expired-at":null`
			if expiredAt != "" {
				expiredField = fmt.Sprintf(`"expired-at":%q`, expiredAt)
			}

			fmt.Fprintf(w, `{
				"data":{
					"id":%q,
					"type":"authentication-tokens",
					"attributes":{
						"created-at":"2026-01-01T00:00:00.000Z",
						"description":"test token",
						%s,
						"last-used-at":"2026-05-13T00:00:00.000Z",
						"token":null
					}
				}
			}`, tokenID, expiredField)
		})
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newStatusClient returns a *client.Client pointed at srv, for use in StatusOpts.
func newStatusClient(t *testing.T, srv *httptest.Server) *client.Client {
	t.Helper()
	c, err := client.New(context.Background(), srv.URL, "test-token", nil)
	require.NoError(t, err)
	return c
}

func TestStatus_Success(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeStatusTFE(t, "testuser", "users", "at-abc123", "2026-06-01T00:00:00.000Z")
	p := profile.TestProfile(t)
	p.Hostname = srv.URL
	p.Token = "test-token"
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)

	opts := &StatusOpts{
		IO:        io,
		Profile:   p,
		Output:    output,
		APIClient: newStatusClient(t, srv),
	}

	r.NoError(runStatus(context.Background(), opts))
	r.Contains(io.Output.String(), "Logged in to")
	r.Contains(io.Output.String(), "testuser")
	r.Contains(io.Output.String(), "Token expires")
}

func TestStatus_Success_NoExpiration(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeStatusTFE(t, "testuser", "users", "at-abc123", "")
	p := profile.TestProfile(t)
	p.Hostname = srv.URL
	p.Token = "test-token"
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)

	opts := &StatusOpts{
		IO:        io,
		Profile:   p,
		Output:    output,
		APIClient: newStatusClient(t, srv),
	}

	r.NoError(runStatus(context.Background(), opts))
	r.Contains(io.Output.String(), "Logged in to")
	r.Contains(io.Output.String(), "testuser")
	r.NotContains(io.Output.String(), "Token expires")
}

func TestStatus_Success_NoTokenLink(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeStatusTFE(t, "testuser", "users", "", "")
	p := profile.TestProfile(t)
	p.Hostname = srv.URL
	p.Token = "test-token"
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)

	opts := &StatusOpts{
		IO:        io,
		Profile:   p,
		Output:    output,
		APIClient: newStatusClient(t, srv),
	}

	r.NoError(runStatus(context.Background(), opts))
	r.Contains(io.Output.String(), "Logged in to")
	r.Contains(io.Output.String(), "testuser")
	r.NotContains(io.Output.String(), "Token expires")
}

func TestStatus_Unauthorized(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeStatusTFE(t, "", "", "", "")
	p := profile.TestProfile(t)
	p.Hostname = srv.URL
	p.Token = "bad-token"
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)

	opts := &StatusOpts{
		IO:        io,
		Profile:   p,
		Output:    output,
		APIClient: newStatusClient(t, srv),
	}

	err := runStatus(context.Background(), opts)
	r.Error(err)
	out := io.Error.String()
	r.Contains(out, "rejected")
	r.Contains(out, "HTTP 401")
	r.Contains(out, srv.URL)
	r.Contains(out, "SSO")
	r.Contains(out, "auth login")
}

func TestStatus_NoToken(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	p := profile.TestProfile(t)
	p.Hostname = "app.terraform.io"
	p.Token = ""
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)

	opts := &StatusOpts{
		IO:      io,
		Profile: p,
		Output:  output,
	}

	err := runStatus(context.Background(), opts)
	r.Error(err)
	out := io.Error.String()
	r.Contains(out, "No token configured")
	r.Contains(out, "app.terraform.io")
	r.Contains(out, "auth login")
}

func TestStatus_Unauthorized_JSON(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeStatusTFE(t, "", "", "", "")
	p := profile.TestProfile(t)
	p.Hostname = srv.URL
	p.Token = "bad-token"
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)
	output.SetFormat(format.JSON)

	opts := &StatusOpts{
		IO:        io,
		Profile:   p,
		Output:    output,
		APIClient: newStatusClient(t, srv),
	}

	err := runStatus(context.Background(), opts)
	r.Error(err)
	out := io.Output.String()
	r.Contains(out, `"active"`)
	r.Contains(out, `"reason"`)
	r.Contains(out, `"rejected"`)
}

func TestClassifyAuthError(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// A 401 means the token itself was rejected.
	f := classifyAuthError(&tfe.APIError{StatusCode: http.StatusUnauthorized})
	r.Equal(reasonRejected, f.reason)
	r.Equal(http.StatusUnauthorized, f.status)

	// The real error is wrapped in a *url.Error; errors.As must still find it.
	wrapped := &url.Error{Op: "Get", URL: "https://tfe.example.com", Err: &tfe.APIError{StatusCode: http.StatusUnauthorized}}
	f = classifyAuthError(wrapped)
	r.Equal(reasonRejected, f.reason)

	// Any other HTTP status is a server-side problem, not an auth failure.
	f = classifyAuthError(&tfe.APIError{StatusCode: http.StatusInternalServerError})
	r.Equal(reasonServerError, f.reason)
	r.Equal(http.StatusInternalServerError, f.status)

	// An error with no HTTP status is a connectivity problem.
	f = classifyAuthError(errors.New("dial tcp: connection refused"))
	r.Equal(reasonUnreachable, f.reason)
}

func TestStatus_JSONOutput(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeStatusTFE(t, "testuser", "users", "at-abc123", "2026-06-01T00:00:00.000Z")
	p := profile.TestProfile(t)
	p.Hostname = srv.URL
	p.Token = "test-token"
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)
	output.SetFormat(format.JSON)

	opts := &StatusOpts{
		IO:        io,
		Profile:   p,
		Output:    output,
		APIClient: newStatusClient(t, srv),
	}

	r.NoError(runStatus(context.Background(), opts))
	r.Contains(io.Output.String(), `"hostname"`)
	r.Contains(io.Output.String(), `"username"`)
	r.Contains(io.Output.String(), `"testuser"`)
	r.Contains(io.Output.String(), `"expires_at"`)
}

func TestStatus_MarkdownOutput(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeStatusTFE(t, "testuser", "users", "at-abc123", "2026-06-01T00:00:00.000Z")
	p := profile.TestProfile(t)
	p.Hostname = srv.URL
	p.Token = "test-token"
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)
	output.SetFormat(format.Markdown)

	opts := &StatusOpts{
		IO:        io,
		Profile:   p,
		Output:    output,
		APIClient: newStatusClient(t, srv),
	}

	r.NoError(runStatus(context.Background(), opts))
	r.Contains(io.Output.String(), "Logged in to")
	r.Contains(io.Output.String(), "testuser")
}

func TestStatus_OrganizationToken(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeStatusTFE(t, "orguser", "organizations", "at-org123", "2026-12-01T00:00:00.000Z")
	p := profile.TestProfile(t)
	p.Hostname = srv.URL
	p.Token = "org-token"
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)

	opts := &StatusOpts{
		IO:        io,
		Profile:   p,
		Output:    output,
		APIClient: newStatusClient(t, srv),
	}

	r.NoError(runStatus(context.Background(), opts))
	r.Contains(io.Output.String(), "organization")
	r.Contains(io.Output.String(), "orguser")
}

func TestStatus_TeamToken(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeStatusTFE(t, "teamuser", "teams", "at-team456", "")
	p := profile.TestProfile(t)
	p.Hostname = srv.URL
	p.Token = "team-token"
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)

	opts := &StatusOpts{
		IO:        io,
		Profile:   p,
		Output:    output,
		APIClient: newStatusClient(t, srv),
	}

	r.NoError(runStatus(context.Background(), opts))
	r.Contains(io.Output.String(), "team")
	r.Contains(io.Output.String(), "teamuser")
}

func TestStatus_QuietMode(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeStatusTFE(t, "testuser", "users", "at-abc123", "2026-06-01T00:00:00.000Z")
	p := profile.TestProfile(t)
	p.Hostname = srv.URL
	p.Token = "test-token"
	r.NoError(p.Write())

	io := iostreams.Test()
	io.SetQuiet(true)
	output := format.New(io)

	opts := &StatusOpts{
		IO:        io,
		Profile:   p,
		Output:    output,
		APIClient: newStatusClient(t, srv),
	}

	r.NoError(runStatus(context.Background(), opts))
	r.Empty(io.Error.String())
	// Output still goes to stdout via displayer
	r.Contains(io.Output.String(), "Logged in to")
}

func TestStatus_DefaultHostname(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	p := profile.TestProfile(t)
	p.Hostname = ""
	p.Token = ""
	r.NoError(p.Write())

	io := iostreams.Test()
	output := format.New(io)

	opts := &StatusOpts{
		IO:      io,
		Profile: p,
		Output:  output,
	}

	err := runStatus(context.Background(), opts)
	r.Error(err)
	r.Contains(io.Error.String(), "app.terraform.io")
}
