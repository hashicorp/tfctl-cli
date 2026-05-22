// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestRunAPI_DefaultGet(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{
						"id":   "ws-1",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "alpha",
						},
					},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces")
	}))
	require.NoError(t, err)

	require.Equal(t, "GET", recorder.Last().Method)
	require.Equal(t, "/api/v2/workspaces", recorder.Last().Path)
	require.Equal(t, "application/vnd.api+json", recorder.Last().Headers.Get("Accept"))
	require.Contains(t, io.Output.String(), "alpha")
	require.Empty(t, io.Error.String())
}

func TestRunAPI_AttributesInferPostAndResourceType(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"POST /api/v2/projects": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{
					"id":   "prj-1",
					"type": "projects",
					"attributes": map[string]any{
						"name": "demo",
					},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/projects")
		opts.Attributes = map[string]string{
			"name": "demo",
		}
	}))
	require.NoError(t, err)

	require.Equal(t, "POST", recorder.Last().Method)
	require.Equal(t, "application/vnd.api+json", recorder.Last().Headers.Get("Content-Type"))
	assertJSONBodyEqual(t, map[string]any{
		"data": map[string]any{
			"type": "projects",
			"attributes": map[string]any{
				"name": "demo",
			},
		},
	}, recorder.Last().JSONBody(t))
	require.Contains(t, io.Output.String(), "Name:")
	require.Contains(t, io.Output.String(), "demo")
}

func TestRunAPI_AttributesInferResourceTypeFromMemberPath(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"POST /api/v2/workspaces/ws-123/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{
					"id":   "var-1",
					"type": "vars",
					"attributes": map[string]any{
						"key": "region",
					},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces/ws-123/vars")
		opts.Attributes = map[string]string{"key": "region"}
	}))
	require.NoError(t, err)
	require.Equal(t, "vars", nestedMap(t, recorder.Last().JSONBody(t), "data")["type"])
}

func TestRunAPI_ExplicitTypeOverridesInferredType(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"POST /api/v2/workspaces/ws-123/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{
					"id":         "custom-1",
					"type":       "custom-vars",
					"attributes": map[string]any{"key": "region"},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces/ws-123/vars")
		opts.Attributes = map[string]string{"key": "region"}
		opts.ResourceType = "custom-vars"
	}))
	require.NoError(t, err)
	require.Equal(t, "custom-vars", nestedMap(t, recorder.Last().JSONBody(t), "data")["type"])
}

func TestRunAPI_InputInlineInferPost(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"POST /api/v2/runs": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{
					"id":         "run-1",
					"type":       "runs",
					"attributes": map[string]any{"message": "queued"},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	input := `{"data":{"type":"runs","attributes":{"message":"queued"}}}`
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/runs")
		opts.InputRequest = input
	}))
	require.NoError(t, err)

	require.Equal(t, "POST", recorder.Last().Method)
	require.Equal(t, "application/vnd.api+json", recorder.Last().Headers.Get("Content-Type"))
	assertJSONBodyEqual(t, jsonMap(t, input), recorder.Last().JSONBody(t))
	require.Contains(t, io.Output.String(), "queued")
}

func TestRunAPI_InputFromStdin(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"POST /api/v2/vars": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{
					"id":         "var-1",
					"type":       "vars",
					"attributes": map[string]any{"key": "AWS_REGION"},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	io.Input.WriteString(`{"data":{"type":"vars","attributes":{"key":"AWS_REGION"}}}`)
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/vars")
		opts.InputRequest = "-"
	}))
	require.NoError(t, err)
	require.Equal(t, "AWS_REGION", nestedMap(t, nestedMap(t, recorder.Last().JSONBody(t), "data"), "attributes")["key"])
}

func TestRunAPI_ExplicitMethodHeadersAndQuery(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"PATCH /api/v2/workspaces/ws-1?include=organization": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"id":         "ws-1",
					"type":       "workspaces",
					"attributes": map[string]any{"name": "beta"},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces/ws-1")
		opts.Method = http.MethodPatch
		opts.Query = map[string]string{"include": "organization"}
		opts.Headers = []string{"X-Test: yes", "Accept: application/custom+json"}
		opts.InputRequest = `{"data":{"type":"workspaces","attributes":{"name":"beta"}}}`
	}))
	require.NoError(t, err)

	req := recorder.Last()
	require.Equal(t, "PATCH", req.Method)
	require.Equal(t, "yes", req.Headers.Get("X-Test"))
	require.Equal(t, "application/custom+json", req.Headers.Get("Accept"))
	require.Equal(t, "organization", req.Query.Get("include"))
}

func TestRunAPI_InlineQueryParamsSparseFieldsets(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/organizations/my-org/workspaces?fields%5Bworkspaces%5D=name": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{
						"id":         "ws-1",
						"type":       "workspaces",
						"attributes": map[string]any{"name": "alpha"},
					},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/organizations/my-org/workspaces?fields[workspaces]=name")
	}))
	require.NoError(t, err)

	req := recorder.Last()
	require.Equal(t, "GET", req.Method)
	require.Equal(t, "/api/v2/organizations/my-org/workspaces", req.Path)
	require.Equal(t, "name", req.Query.Get("fields[workspaces]"))
}

func TestRunAPI_InlineQueryParamsMergedWithFlags(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces?fields%5Bworkspaces%5D=name&include=organization": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{
						"id":         "ws-1",
						"type":       "workspaces",
						"attributes": map[string]any{"name": "alpha"},
					},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces?fields[workspaces]=name")
		opts.Query = map[string]string{"include": "organization"}
	}))
	require.NoError(t, err)

	req := recorder.Last()
	require.Equal(t, "name", req.Query.Get("fields[workspaces]"))
	require.Equal(t, "organization", req.Query.Get("include"))
}

func TestRunAPI_Paginate(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{"id": "ws-1", "type": "workspaces", "attributes": map[string]any{"name": "alpha"}},
				},
				"links": map[string]any{"next": serverURL(r) + "/api/v2/workspaces?page[number]=2"},
				"meta":  map[string]any{"pagination": map[string]any{"total-count": 1}},
			})
		},
		"GET /api/v2/workspaces?page[number]=2": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{"id": "ws-2", "type": "workspaces", "attributes": map[string]any{"name": "beta"}},
				},
				"links": map[string]any{"next": nil},
				"meta":  map[string]any{"pagination": map[string]any{"total-count": 2}},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces")
		opts.All = true
	}))
	require.NoError(t, err)

	require.Len(t, recorder.All(), 2)
	require.Contains(t, io.Output.String(), "alpha")
	require.Contains(t, io.Output.String(), "beta")
}

func TestRunAPI_PageNumber(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces?page%5Bnumber%5D=2": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{"id": "ws-2", "type": "workspaces", "attributes": map[string]any{"name": "beta"}},
				},
				"links": map[string]any{"next": nil},
				"meta":  map[string]any{"pagination": map[string]any{"total-count": 2}},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces")
		opts.PageNumber = 2
	}))
	require.NoError(t, err)

	require.Len(t, recorder.All(), 1)
	require.Contains(t, io.Output.String(), "beta")
}

func TestRunAPI_PageSize(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces?page%5Bsize%5D=1": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{"id": "ws-1", "type": "workspaces", "attributes": map[string]any{"name": "alpha"}},
				},
				"links": map[string]any{"next": "/api/v2/workspaces?page[size]=1&page[number]=2"},
				"meta":  map[string]any{"pagination": map[string]any{"total-count": 2}},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces")
		opts.PageSize = 1
	}))
	require.NoError(t, err)

	require.Len(t, recorder.All(), 1)
	require.Contains(t, io.Output.String(), "alpha")
}

func TestMergePaginatedBody_UpdatesMeta(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"data": [{"id": "ws-2", "type": "workspaces"}],
		"meta": {"pagination": {"current-page": 2, "page-size": 1, "total-count": 2, "total-pages": 2}},
		"links": {"next": "https://example.com/next"}
	}`)
	combined := []any{
		map[string]any{"id": "ws-1", "type": "workspaces"},
		map[string]any{"id": "ws-2", "type": "workspaces"},
	}

	merged, err := mergePaginatedBody(body, combined)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(merged, &result))

	data := result["data"].([]any)
	require.Len(t, data, 2)

	meta := result["meta"].(map[string]any)
	pagination := meta["pagination"].(map[string]any)
	require.EqualValues(t, 2, pagination["total-count"])
	require.EqualValues(t, 2, pagination["page-size"])
	require.EqualValues(t, 1, pagination["current-page"])
	require.EqualValues(t, 1, pagination["total-pages"])

	links := result["links"].(map[string]any)
	require.Nil(t, links["next"])
}

func TestRunAPI_QuietSuppressesOutput(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{map[string]any{"id": "ws-1", "type": "workspaces", "attributes": map[string]any{"name": "alpha"}}},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces")
		opts.Quiet = true
	}))
	require.NoError(t, err)
	require.Empty(t, io.Output.String())
}

func TestRunAPI_EmptyBodyNoOutput(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"DELETE /api/v2/workspaces/ws-1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.api+json")
			w.WriteHeader(http.StatusNoContent)
		},
	})
	defer server.Close()

	io := iostreams.Test()
	io.ErrorTTY = true
	io.InputTTY = true
	io.Input.Write([]byte("y\n"))

	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces/ws-1")
		opts.Method = http.MethodDelete
	}))
	require.NoError(t, err)
	require.Empty(t, io.Output.String())
}

func TestRunAPI_DeleteNoConfirmation(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"DELETE /api/v2/workspaces/ws-1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.api+json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{ \"data\": { \"attributes\": { \"hello\": \"world\" } } }"))
		},
	})
	defer server.Close()

	io := iostreams.Test()
	io.ErrorTTY = true
	io.InputTTY = true
	io.Input.Write([]byte("n\n"))

	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces/ws-1")
		opts.Method = http.MethodDelete
	}))
	require.ErrorContains(t, err, "DELETE request canceled")
}

func TestRunAPI_DeleteQuietMode(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"DELETE /api/v2/workspaces/ws-1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.api+json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{ \"data\": { \"attributes\": { \"hello\": \"world\" } } }"))
		},
	})
	defer server.Close()

	io := iostreams.Test()
	io.ErrorTTY = true
	io.InputTTY = true
	io.SetQuiet(true)
	io.Input.Write([]byte("y\n"))

	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces/ws-1")
		opts.Method = http.MethodDelete
		opts.Quiet = true
	}))
	require.ErrorContains(t, err, "can't perform DELETE request confirmation with quiet mode enabled")
}

func TestRunAPI_ErrorResponseSummarizesJSONAPIErrors(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusUnprocessableEntity, map[string]any{
				"errors": []any{
					map[string]any{"title": "Validation failed", "detail": "name is required"},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces")
	}))
	assert.ErrorIs(t, err, tfe.ErrUnprocessableEntity)
	var apiErr *tfe.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Len(t, apiErr.Details, 1)
	require.Equal(t, apiErr.Details[0], "Validation failed: name is required")
}

func TestRunAPI_ErrorResponseFallsBackToRawBody(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "bad request !!!")
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces")
	}))
	assert.ErrorIs(t, err, tfe.ErrBadRequest)
	var apiErr *tfe.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Len(t, apiErr.Details, 0)
	require.Equal(t, apiErr.Error(), "400 Bad Request")
}

func TestRunAPI_ErrorResponseHTML(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<html>Not found</html>")
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces")
	}))
	require.EqualError(t, err, "404 Not Found")
}

func TestRunAPI_PaginateReturnsErrorResponseFromLaterPage(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data":  []any{map[string]any{"id": "ws-1", "type": "workspaces", "attributes": map[string]any{"name": "alpha"}}},
				"links": map[string]any{"next": serverURL(r) + "/api/v2/workspaces?page[number]=2"},
			})
		},
		"GET /api/v2/workspaces?page[number]=2": func(w http.ResponseWriter, r *http.Request) {
			writeJSONAPIResponse(w, http.StatusTooManyRequests, map[string]any{
				"errors": []any{map[string]any{"title": "Rate limit", "detail": "slow down"}},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces")
		opts.All = true
	}))
	assert.ErrorIs(t, err, tfe.ErrTooManyRequests)
	var apiErr *tfe.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Len(t, apiErr.Details, 1)
	require.Equal(t, apiErr.Details[0], "Rate limit: slow down")
}

func TestRunAPI_InvalidHeaderReturnsError(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	err := runAPI(newTestOpts(t, "https://example.test", io, func(opts *Opts) {
		opts.URL = mustParseURL(t, "https://example.test/api/v2/workspaces")
		opts.Headers = []string{"invalid-header"}
	}))
	require.EqualError(t, err, `invalid pair "invalid-header"`)
}

type recordedRequest struct {
	Method  string
	Path    string
	Query   url.Values
	Headers http.Header
	Body    []byte
}

func (r recordedRequest) JSONBody(t *testing.T) map[string]any {
	t.Helper()
	if len(r.Body) == 0 {
		return nil
	}
	var body map[string]any
	require.NoError(t, json.Unmarshal(r.Body, &body))
	return body
}

type requestRecorder struct {
	mu       sync.Mutex
	requests []recordedRequest
}

func (r *requestRecorder) Record(req recordedRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
}

func (r *requestRecorder) Last() recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests[len(r.requests)-1]
}

func (r *requestRecorder) All() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]recordedRequest, len(r.requests))
	copy(requests, r.requests)
	return requests
}

func newAPITestServer(routes map[string]http.HandlerFunc) (*httptest.Server, *requestRecorder) {
	recorder := &requestRecorder{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		recorder.Record(recordedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Query:   r.URL.Query(),
			Headers: r.Header.Clone(),
			Body:    body,
		})

		if h, ok := routes[routeKey(r)]; ok {
			h(w, r)
			return
		}

		http.Error(w, "unexpected request "+routeKey(r), http.StatusInternalServerError)
	})
	return httptest.NewServer(handler), recorder
}

func routeKey(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	}
	return fmt.Sprintf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
}

func newTestOpts(t *testing.T, address string, io *iostreams.Testing, mutate func(*Opts)) *Opts {
	t.Helper()
	apiClient, err := client.New(address, "test-token", http.Header{
		"User-Agent": []string{"tfctl-cli/test"},
	})
	require.NoError(t, err)

	opts := &Opts{
		IO:          io,
		Logger:      hclog.NewNullLogger(),
		Output:      format.New(io),
		ShutdownCtx: context.Background(),
		Client:      apiClient,
		Headers:     []string{},
		Attributes:  map[string]string{},
		Query:       map[string]string{},
		PathParams:  map[string]string{},
	}
	mutate(opts)
	return opts
}

func mustResolveTestURL(t *testing.T, base, path string) *url.URL {
	t.Helper()
	baseURL := mustParseURL(t, base)
	resolved, err := client.ResolveURL(*baseURL, path)
	require.NoError(t, err)
	return resolved
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func assertJSONBodyEqual(t *testing.T, want, got map[string]any) {
	t.Helper()
	wantJSON, err := json.Marshal(want)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, string(wantJSON), string(gotJSON))
}

func jsonMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	return payload
}

func nestedMap(t *testing.T, payload map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := payload[key].(map[string]any)
	require.True(t, ok, "expected map at key %q, got %T", key, payload[key])
	return value
}

func writeJSONAPIResponse(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func TestLookupResource_ErrNotFound(t *testing.T) {
	t.Parallel()

	// Mock server that returns 404 for workspace lookup.
	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/organizations/my-org/workspaces/nonexistent": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.api+json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errors": []map[string]any{
					{"status": "404", "title": "not found"},
				},
			})
		},
	})
	defer server.Close()

	apiClient, err := client.New(server.URL, "test-token", http.Header{})
	require.NoError(t, err)

	resolver := client.NewResolver(apiClient, false, false)
	_, err = lookupResource(context.Background(), resolver, "workspaces", "my-org", "nonexistent")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `workspaces named "nonexistent" not found in organization "my-org"`)
}

func TestWriteDryRunRequest(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	u, err := url.Parse("https://example.com/api/v2/projects")
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{
		"Accept":       []string{"application/vnd.api+json"},
		"Content-Type": []string{"application/vnd.api+json"},
	}
	body := []byte(`{"data":{"type":"projects"}}`)

	writeDryRunRequest(io.Err(), http.MethodPost, u, headers, body)

	output := io.Error.String()
	if !strings.Contains(output, "> POST https://example.com/api/v2/projects") {
		t.Fatalf("expected request line, got %q", output)
	}
	if !strings.Contains(output, `"type": "projects"`) {
		t.Fatalf("expected body, got %q", output)
	}
}
