// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/telemetry"
)

func TestResolveURL(t *testing.T) {
	t.Parallel()

	base := url.URL{
		Scheme: "https",
		Host:   "app.terraform.io",
		Path:   "/api/v2",
	}

	tests := []struct {
		name        string
		path        string
		wantPath    string
		wantRawPath string
		wantRaw     string
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
		{
			name:        "percent-encoded path segment",
			path:        "/workspaces/my%2Fworkspace/runs",
			wantPath:    "/api/v2/workspaces/my/workspace/runs",
			wantRawPath: "/api/v2/workspaces/my%2Fworkspace/runs",
			wantRaw:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveURL(base, tt.path)
			require.NoError(t, err)
			require.Equal(t, tt.wantPath, got.Path)
			require.Equal(t, tt.wantRawPath, got.RawPath)
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

func TestNew(t *testing.T) {
	t.Parallel()

	defaultHeaders := http.Header{
		"X-Test-Header": []string{"test-value"},
	}

	client, err := New(context.Background(), "https://app.terraform.io", "test-token", defaultHeaders)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, client.TFE)
	require.NotNil(t, client.Adapter)
	require.Equal(t, "https", client.BaseURL.Scheme)
	require.Equal(t, "app.terraform.io", client.BaseURL.Host)
	require.Equal(t, "/api/v2", client.BaseURL.Path)
	require.Equal(t, defaultHeaders, client.DefaultHeaders)
}

func TestClientDo(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	requestURL, err := ResolveURL(*client.BaseURL, "/workspaces")
	require.NoError(t, err)

	var gotRequest *http.Request
	var gotBody []byte
	client.Adapter.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotRequest = req.Clone(req.Context())

		if req.Body != nil {
			gotBody, err = io.ReadAll(req.Body)
			require.NoError(t, err)
		}

		return &http.Response{
			Status:        "200 OK",
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          io.NopCloser(strings.NewReader(`{"ok":true}`)),
			ContentLength: int64(len(`{"ok":true}`)),
			Request:       req,
		}, nil
	})

	resp, err := client.Do(context.Background(), &Request{
		Method: http.MethodPost,
		URL:    requestURL,
		Headers: http.Header{
			"X-Request-Header": []string{"request-value"},
		},
		Body: []byte(`{"hello":"world"}`),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, resp.Body.Close())
	})

	require.NotNil(t, gotRequest)
	require.Equal(t, http.MethodPost, gotRequest.Method)
	require.Equal(t, requestURL.String(), gotRequest.URL.String())
	require.Equal(t, "default-value", gotRequest.Header.Get("X-Default-Header"))
	require.Equal(t, "request-value", gotRequest.Header.Get("X-Request-Header"))
	require.Equal(t, `{"hello":"world"}`, string(gotBody))
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestClientDo_UnwrapsTransportErrors(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	requestURL, err := ResolveURL(*client.BaseURL, "/workspaces")
	require.NoError(t, err)

	wantErr := errors.New("transport failed")
	client.Adapter.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, wantErr
	})

	resp, err := client.Do(context.Background(), &Request{
		Method: http.MethodGet,
		URL:    requestURL,
	})
	require.Nil(t, resp)
	require.ErrorIs(t, err, wantErr)
}

func TestClientSetLogger_Response(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	requestURL, err := ResolveURL(*client.BaseURL, "/workspaces")
	require.NoError(t, err)

	var logs bytes.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Level:  hclog.Debug,
		Output: &logs,
		Color:  hclog.ColorOff,
	})

	client.Adapter.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:        "200 OK",
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          io.NopCloser(strings.NewReader(`{"ok":true}`)),
			ContentLength: int64(len(`{"ok":true}`)),
			Request:       req,
		}, nil
	})
	client.SetLogger(logger)

	resp, err := client.Do(context.Background(), &Request{Method: http.MethodGet, URL: requestURL})
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	logOutput := logs.String()
	require.Contains(t, logOutput, "HTTP request")
	require.Contains(t, logOutput, "HTTP response")
	require.Contains(t, logOutput, requestURL.String())
	require.Contains(t, logOutput, "200 OK")
}

func TestClientSetLogger_Error(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	requestURL, err := ResolveURL(*client.BaseURL, "/workspaces")
	require.NoError(t, err)

	var logs bytes.Buffer
	logger := hclog.New(&hclog.LoggerOptions{
		Level:  hclog.Debug,
		Output: &logs,
		Color:  hclog.ColorOff,
	})

	wantErr := errors.New("request failed")
	client.Adapter.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, wantErr
	})
	client.SetLogger(logger)

	resp, err := client.Do(context.Background(), &Request{Method: http.MethodGet, URL: requestURL})
	require.Nil(t, resp)
	require.ErrorIs(t, err, wantErr)

	logOutput := logs.String()
	require.Contains(t, logOutput, "HTTP request")
	require.Contains(t, logOutput, "HTTP request error")
	require.Contains(t, logOutput, wantErr.Error())
}

func TestClientSetTelemetry_Response(t *testing.T) {
	var traceOutput bytes.Buffer
	tel := telemetry.Init(context.Background(), telemetry.Config{
		ProfileTelemetry: "log",
		ErrWriter:        &traceOutput,
		Version:          "test",
	})

	client := newTestClient(t)
	requestURL, err := ResolveURL(*client.BaseURL, "/workspaces")
	require.NoError(t, err)

	client.Adapter.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:        "200 OK",
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          io.NopCloser(strings.NewReader(`{"ok":true}`)),
			ContentLength: int64(len(`{"ok":true}`)),
			Request:       req,
		}, nil
	})
	client.SetTelemetry(tel)

	resp, err := client.Do(context.Background(), &Request{Method: http.MethodGet, URL: requestURL})
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.NoError(t, tel.Shutdown(context.Background(), 0))

	output := traceOutput.String()
	require.Contains(t, output, "client req")
	require.Contains(t, output, "http.status_code")
	require.Contains(t, output, requestURL.Path)
}

func TestClientSetTelemetry_Error(t *testing.T) {
	var traceOutput bytes.Buffer
	tel := telemetry.Init(context.Background(), telemetry.Config{
		ProfileTelemetry: "log",
		ErrWriter:        &traceOutput,
		Version:          "test",
	})

	client := newTestClient(t)
	requestURL, err := ResolveURL(*client.BaseURL, "/workspaces")
	require.NoError(t, err)

	wantErr := errors.New("telemetry transport failed")
	client.Adapter.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, wantErr
	})
	client.SetTelemetry(tel)

	resp, err := client.Do(context.Background(), &Request{Method: http.MethodGet, URL: requestURL})
	require.Nil(t, resp)
	require.ErrorIs(t, err, wantErr)
	require.NoError(t, tel.Shutdown(context.Background(), 0))

	output := traceOutput.String()
	require.Contains(t, output, "client req")
	require.Contains(t, output, requestURL.Path)
	require.Contains(t, output, wantErr.Error())
}

func newTestClient(t *testing.T) *Client {
	t.Helper()

	client, err := New(context.Background(), "https://app.terraform.io", "test-token", http.Header{
		"X-Default-Header": []string{"default-value"},
	})
	require.NoError(t, err)

	return client
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
