// Copyright IBM Corp. 2026

package create

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestRunCreate(t *testing.T) {
	t.Parallel()

	t.Run("create workspace with attributes", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		var receivedBody map[string]any
		ctx := testContext(t, io, testServer(t, routeMap{
			"POST /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-new123",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "foo",
						},
					},
				})
			},
		}))
		ctx.Profile.Organization = "my-org"

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"name": "foo"},
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-new123")

		// Verify request body structure
		data := receivedBody["data"].(map[string]any)
		assert.Equal(t, "workspaces", data["type"])
		attrs := data["attributes"].(map[string]any)
		assert.Equal(t, "foo", attrs["name"])
	})

	t.Run("create workspace with input body", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		var receivedBody map[string]any
		ctx := testContext(t, io, testServer(t, routeMap{
			"POST /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-input123",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "from-input",
						},
					},
				})
			},
		}))
		ctx.Profile.Organization = "my-org"

		inputJSON := `{"data":{"type":"workspaces","attributes":{"name":"from-input"}}}`
		err := runCreate(ctx, &Opts{
			InputBody: inputJSON,
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-input123")
	})

	t.Run("create workspace dry run", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))
		ctx.Profile.Organization = "my-org"

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"name": "foo"},
			DryRun:     true,
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.NoError(t, err)
		// Dry run should show the request info on stderr
		assert.Contains(t, io.Error.String(), "POST")
	})

	t.Run("create with no attrs and no input errors", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))
		ctx.Profile.Organization = "my-org"

		err := runCreate(ctx, &Opts{}, hclog.NewNullLogger(), []string{"workspace"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provide attributes with -a key=value or a request body with -i")
	})

	t.Run("create unsupported resource type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"x": "y"},
		}, hclog.NewNullLogger(), []string{"apply"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create is not supported for applies")
	})

	t.Run("create unknown resource type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"x": "y"},
		}, hclog.NewNullLogger(), []string{"blah"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource type")
		assert.Contains(t, err.Error(), "Available resources:")
	})

	t.Run("create workspace without org errors", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"name": "foo"},
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "organization is required but not set")
	})

	t.Run("no args returns usage", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))

		err := runCreate(ctx, &Opts{}, hclog.NewNullLogger(), []string{})
		require.ErrorIs(t, err, cmd.ErrDisplayUsage)
	})

	t.Run("explicit org flag overrides profile", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{
			"POST /api/v2/organizations/flag-org/workspaces": func(w http.ResponseWriter, _ *http.Request) {
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-flagorg",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "created-in-flag-org",
						},
					},
				})
			},
		}))
		ctx.Profile.Organization = "profile-org"

		err := runCreate(ctx, &Opts{
			Organization: "flag-org",
			Attributes:   map[string]string{"name": "created-in-flag-org"},
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-flagorg")
	})
}

// --- test helpers ---

type routeMap map[string]http.HandlerFunc

func (rm routeMap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	if h, ok := rm[key]; ok {
		h(w, r)
		return
	}
	http.Error(w, "unexpected: "+key, http.StatusInternalServerError)
}

func testServer(t *testing.T, routes routeMap) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return server
}

func testContext(t *testing.T, io *iostreams.Testing, server *httptest.Server) *cmd.Context {
	t.Helper()
	p := profile.TestProfile(t)
	p.Hostname = server.URL
	return &cmd.Context{
		IO:          io,
		Output:      format.New(io),
		ShutdownCtx: context.Background(),
		Profile:     p,
	}
}

func jsonapi(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	_ = json.NewEncoder(w).Encode(payload)
}
