// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

// TestRunAPI_PaginateSinglePage covers the case where --all is set but the
// result fits in a single page (no "next" link). The response body must still
// be rendered; a single-page result must not come back empty just because
// pagination was requested.
func TestRunAPI_PaginateSinglePage(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/workspaces": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": []any{
					map[string]any{"id": "ws-1", "type": "workspaces", "attributes": map[string]any{"name": "alpha"}},
				},
				"links": map[string]any{"next": nil},
				"meta":  map[string]any{"pagination": map[string]any{"total-count": 1}},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := RunAPI(context.Background(), newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces")
		opts.All = true
	}))
	require.NoError(t, err)

	require.Len(t, recorder.All(), 1)
	require.Contains(t, io.Output.String(), "alpha",
		"single-page --all result must still be rendered, got: %q", io.Output.String())
	require.Empty(t, io.Error.String())
}
