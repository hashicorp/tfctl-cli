// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package variable

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/client"
	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
)

func TestRunVariable_ImportWorkspaceFromTFVarsFile(t *testing.T) {
	t.Parallel()

	server, recorder := newVariableImportTestServer(map[string]http.HandlerFunc{
		"GET /api/v2/organizations/test-org/workspaces/test-workspace": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": map[string]any{"id": "ws-123", "type": "workspaces"},
			})
		},
		"GET /api/v2/workspaces/ws-123/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{"data": []any{}})
		},
		"POST /api/v2/workspaces/ws-123/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{"id": "var-created", "type": "vars"},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	tfvars := writeTestTFVarsFile(t, "example = \"value\"\ncount = 3\n")

	err := runVariableImport(newImportTestOpts(t, server.URL, io, func(opts *ImportOpts) {
		opts.Organization = "test-org"
		opts.Workspace = "test-workspace"
		opts.TFVarsFileToImport = tfvars
	}))
	require.NoError(t, err)

	reqs := recorder.All()
	require.Len(t, reqs, 4)
	require.Equal(t, "GET", reqs[0].Method)
	require.Equal(t, "/api/v2/organizations/test-org/workspaces/test-workspace", reqs[0].Path)
	require.Equal(t, "GET", reqs[1].Method)
	require.Equal(t, "/api/v2/workspaces/ws-123/vars", reqs[1].Path)
	requireRequestPayloads(t, reqs[2:], "POST", "/api/v2/workspaces/ws-123/vars", []map[string]any{
		{
			"data": map[string]any{
				"attributes": map[string]any{
					"category":  "terraform",
					"hcl":       false,
					"key":       "example",
					"sensitive": false,
					"value":     "value",
				},
			},
		},
		{
			"data": map[string]any{
				"attributes": map[string]any{
					"category":  "terraform",
					"hcl":       false,
					"key":       "count",
					"sensitive": false,
					"value":     "3",
				},
			},
		},
	})

	require.Equal(t, fmt.Sprintf("%s imported 2 variables into workspace \"test-workspace\" (2 created, 0 updated)", io.ColorScheme().SuccessIcon()), io.Error.String())
}

func TestRunVariable_ImportVariableSetFromEnv(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "secret-value")

	server, recorder := newVariableImportTestServer(map[string]http.HandlerFunc{
		"GET /api/v2/organizations/test-org/varsets?q=my-set": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{
						"id":         "vs-123",
						"type":       "varsets",
						"attributes": map[string]any{"name": "my-set"},
					},
				},
			})
		},
		"GET /api/v2/varsets/vs-123/relationships/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{"data": []any{}})
		},
		"POST /api/v2/varsets/vs-123/relationships/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{"id": "var-created", "type": "vars"},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runVariableImport(newImportTestOpts(t, server.URL, io, func(opts *ImportOpts) {
		opts.Organization = "test-org"
		opts.VariableSetName = "my-set"
		opts.Env = []string{"AWS_ACCESS_KEY_ID"}
	}))
	require.NoError(t, err)

	reqs := recorder.All()
	require.Len(t, reqs, 3)
	payload := jsonMap(t, string(reqs[2].Body))
	require.Equal(t, map[string]any{
		"data": map[string]any{
			"attributes": map[string]any{
				"category":  "env",
				"hcl":       false,
				"key":       "AWS_ACCESS_KEY_ID",
				"sensitive": true,
				"value":     "secret-value",
			},
		},
	}, payload)
	require.Equal(t, fmt.Sprintf("%s imported 1 variables into variable set \"my-set\" (1 created, 0 updated)", io.ColorScheme().SuccessIcon()), io.Error.String())
}

func TestRunVariable_ImportReturnsUsageWhenNothingToImport(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	err := runVariableImport(&ImportOpts{IO: io})
	require.ErrorIs(t, err, cmd.ErrDisplayUsage)
	require.Empty(t, io.Error.String())
}

func TestRunVariable_ImportErrorsWhenEnvVarMissing(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	err := runVariableImport(&ImportOpts{
		IO:  io,
		Env: []string{"MISSING_ENV"},
	})
	require.EqualError(t, err, "environment variable \"MISSING_ENV\" is not set")
	require.Empty(t, io.Error.String())
}

func TestRunVariable_ImportErrorsOnDuplicateWithoutOverwrite(t *testing.T) {
	t.Setenv("DUPLICATE_ENV", "new-value")

	server, recorder := newVariableImportTestServer(map[string]http.HandlerFunc{
		"GET /api/v2/organizations/test-org/workspaces/test-workspace": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": map[string]any{"id": "ws-123", "type": "workspaces"},
			})
		},
		"GET /api/v2/workspaces/ws-123/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{
						"id":   "var-1",
						"type": "vars",
						"attributes": map[string]any{
							"key":      "DUPLICATE_ENV",
							"category": "env",
						},
					},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runVariableImport(newImportTestOpts(t, server.URL, io, func(opts *ImportOpts) {
		opts.Organization = "test-org"
		opts.Workspace = "test-workspace"
		opts.Env = []string{"DUPLICATE_ENV"}
	}))
	require.EqualError(t, err, "variables already exist; rerun with --overwrite to update: DUPLICATE_ENV (env)")
	require.Len(t, recorder.All(), 2)
	require.Empty(t, io.Error.String())
}

func TestRunVariable_ImportOverwriteUpdatesExistingVariables(t *testing.T) {
	t.Setenv("AWS_SECRET_ACCESS_KEY", "updated-secret")

	server, recorder := newVariableImportTestServer(map[string]http.HandlerFunc{
		"GET /api/v2/organizations/test-org/varsets?q=my-set": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{
						"id":         "vs-123",
						"type":       "varsets",
						"attributes": map[string]any{"name": "my-set"},
					},
				},
			})
		},
		"GET /api/v2/varsets/vs-123/relationships/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{
						"id":   "var-99",
						"type": "vars",
						"attributes": map[string]any{
							"key":      "AWS_SECRET_ACCESS_KEY",
							"category": "env",
						},
					},
				},
			})
		},
		"PATCH /api/v2/varsets/vs-123/relationships/vars/var-99": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": map[string]any{"id": "var-99", "type": "vars"},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runVariableImport(newImportTestOpts(t, server.URL, io, func(opts *ImportOpts) {
		opts.Organization = "test-org"
		opts.VariableSetName = "my-set"
		opts.Env = []string{"AWS_SECRET_ACCESS_KEY"}
		opts.Overwrite = true
	}))
	require.NoError(t, err)

	reqs := recorder.All()
	require.Len(t, reqs, 3)
	payload := jsonMap(t, string(reqs[2].Body))
	require.Equal(t, map[string]any{
		"data": map[string]any{
			"attributes": map[string]any{
				"category":  "env",
				"hcl":       false,
				"key":       "AWS_SECRET_ACCESS_KEY",
				"sensitive": true,
				"value":     "updated-secret",
			},
		},
	}, payload)
	require.Equal(t, fmt.Sprintf("%s imported 1 variables into variable set \"my-set\" (0 created, 1 updated)", io.ColorScheme().SuccessIcon()), io.Error.String())
}

type variableImportRecordedRequest struct {
	Method  string
	Path    string
	Query   string
	Headers http.Header
	Body    []byte
}

type variableImportRequestRecorder struct {
	mu       sync.Mutex
	requests []variableImportRecordedRequest
}

func (r *variableImportRequestRecorder) Record(req variableImportRecordedRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
}

func (r *variableImportRequestRecorder) All() []variableImportRecordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]variableImportRecordedRequest, len(r.requests))
	copy(requests, r.requests)
	return requests
}

func newVariableImportTestServer(routes map[string]http.HandlerFunc) (*httptest.Server, *variableImportRequestRecorder) {
	recorder := &variableImportRequestRecorder{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		recorder.Record(variableImportRecordedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Query:   r.URL.RawQuery,
			Headers: r.Header.Clone(),
			Body:    body,
		})

		key := variableImportRouteKey(r)
		if h, ok := routes[key]; ok {
			h(w, r)
			return
		}

		http.Error(w, "unexpected request "+key, http.StatusInternalServerError)
	})

	return httptest.NewServer(handler), recorder
}

func variableImportRouteKey(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	}
	return fmt.Sprintf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
}

func newImportTestOpts(t *testing.T, address string, io *iostreams.Testing, mutate func(*ImportOpts)) *ImportOpts {
	t.Helper()
	apiClient, err := client.New(address, "test-token", http.Header{
		"User-Agent": []string{"tfcloud-cli/test"},
	})
	require.NoError(t, err)

	opts := &ImportOpts{
		IO:          io,
		ShutdownCtx: context.Background(),
		Client:      apiClient,
	}
	if mutate != nil {
		mutate(opts)
	}
	return opts
}

func writeTestTFVarsFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "variables.tfvars")
	err := os.WriteFile(path, []byte(contents), 0o600)
	require.NoError(t, err)
	return path
}

func jsonMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	return payload
}

func requireRequestPayloads(t *testing.T, reqs []variableImportRecordedRequest, method, path string, want []map[string]any) {
	t.Helper()

	got := make([]string, 0, len(reqs))
	for _, req := range reqs {
		require.Equal(t, method, req.Method)
		require.Equal(t, path, req.Path)

		payload, err := json.Marshal(jsonMap(t, string(req.Body)))
		require.NoError(t, err)
		got = append(got, string(payload))
	}

	wantPayloads := make([]string, 0, len(want))
	for _, payload := range want {
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		wantPayloads = append(wantPayloads, string(encoded))
	}

	require.ElementsMatch(t, wantPayloads, got)
}

func writeJSONAPIResponse(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func TestRunVariable_ImportParsesJSONTFVarsFile(t *testing.T) {
	t.Parallel()

	server, recorder := newVariableImportTestServer(map[string]http.HandlerFunc{
		"GET /api/v2/organizations/test-org/workspaces/test-workspace": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": map[string]any{"id": "ws-123", "type": "workspaces"},
			})
		},
		"GET /api/v2/workspaces/ws-123/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{"data": []any{}})
		},
		"POST /api/v2/workspaces/ws-123/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{"id": "var-created", "type": "vars"},
			})
		},
	})
	defer server.Close()

	path := filepath.Join(t.TempDir(), "variables.tfvars.json")
	err := os.WriteFile(path, []byte(`{"enabled":true}`), 0o600)
	require.NoError(t, err)

	io := iostreams.Test()
	err = runVariableImport(newImportTestOpts(t, server.URL, io, func(opts *ImportOpts) {
		opts.Organization = "test-org"
		opts.Workspace = "test-workspace"
		opts.TFVarsFileToImport = path
	}))
	require.NoError(t, err)

	reqs := recorder.All()
	require.Len(t, reqs, 3)
	require.True(t, strings.Contains(string(reqs[2].Body), `"key":"enabled"`))
	require.True(t, strings.Contains(string(reqs[2].Body), `"value":"true"`))
}
