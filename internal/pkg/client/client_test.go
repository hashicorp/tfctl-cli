// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveURL(t *testing.T) {
	t.Parallel()

	base := url.URL{
		Scheme: "https",
		Host:   "app.terraform.io",
		Path:   "/api/v2",
	}

	tests := []struct {
		name     string
		path     string
		wantPath string
		wantRaw  string
	}{
		{
			name:     "simple path",
			path:     "/workspaces",
			wantPath: "/api/v2/workspaces",
			wantRaw:  "",
		},
		{
			name:     "path without leading slash",
			path:     "workspaces",
			wantPath: "/api/v2/workspaces",
			wantRaw:  "",
		},
		{
			name:     "sparse fieldsets",
			path:     "/organizations/my-org/workspaces?fields[workspaces]=name",
			wantPath: "/api/v2/organizations/my-org/workspaces",
			wantRaw:  "fields%5Bworkspaces%5D=name",
		},
		{
			name:     "multiple query params",
			path:     "/workspaces?include=organization&fields[workspaces]=name,vcs-repo",
			wantPath: "/api/v2/workspaces",
			wantRaw:  "fields%5Bworkspaces%5D=name%2Cvcs-repo&include=organization",
		},
		{
			name:     "pagination query params",
			path:     "/workspaces?page[number]=2&page[size]=50",
			wantPath: "/api/v2/workspaces",
			wantRaw:  "page%5Bnumber%5D=2&page%5Bsize%5D=50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveURL(base, tt.path)
			require.NoError(t, err)
			require.Equal(t, tt.wantPath, got.Path)
			require.Equal(t, tt.wantRaw, got.Query().Encode())
		})
	}
}

func TestResolveURL_AbsoluteURL(t *testing.T) {
	t.Parallel()

	base := url.URL{
		Scheme: "https",
		Host:   "app.terraform.io",
		Path:   "/api/v2",
	}

	got, err := ResolveURL(base, "https://other.host/api/v2/workspaces?fields[workspaces]=name")
	require.NoError(t, err)
	require.Equal(t, "other.host", got.Host)
	require.Equal(t, "/api/v2/workspaces", got.Path)
	require.Equal(t, "fields%5Bworkspaces%5D=name", got.Query().Encode())
}
