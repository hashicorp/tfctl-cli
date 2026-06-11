// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package cmdtest provides shared test helpers for command tests.
package cmdtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

// RouteMap maps "METHOD /path" to a handler function for use in test servers.
type RouteMap map[string]http.HandlerFunc

// ServeHTTP dispatches to the matching route handler. It tries an exact match
// including query params first, then falls back to path-only matching.
func (rm RouteMap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try exact match with query params first.
	if r.URL.RawQuery != "" {
		keyWithQuery := fmt.Sprintf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		if h, ok := rm[keyWithQuery]; ok {
			h(w, r)
			return
		}
	}
	// Fall back to path-only match.
	key := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	if h, ok := rm[key]; ok {
		h(w, r)
		return
	}
	http.Error(w, "unexpected: "+key, http.StatusInternalServerError)
}

// NewServer creates an httptest.Server with the given routes and registers cleanup.
func NewServer(t *testing.T, routes RouteMap) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return server
}

// NewContext creates a cmd.Context suitable for testing, with a profile
// pointed at the given test server.
func NewContext(t *testing.T, io *iostreams.Testing, server *httptest.Server) *cmd.Context {
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

// WriteJSONAPI writes a JSON:API response with the correct content type.
func WriteJSONAPI(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	_ = json.NewEncoder(w).Encode(payload)
}
