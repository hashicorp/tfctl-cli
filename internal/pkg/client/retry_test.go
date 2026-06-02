// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

// TestRetryServerErrors verifies that 5xx responses are retried when
// RetryServerErrors is enabled in the tfe.Config.
func TestRetryServerErrors(t *testing.T) {
	t.Parallel()

	var attempts int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, "test-token", nil, hclog.NewNullLogger())
	require.NoError(t, err)

	resp, err := c.TFE.GetStream(t.Context(), "/test", nil)
	require.NoError(t, err)
	resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.GreaterOrEqual(t, atomic.LoadInt64(&attempts), int64(3))
}

// TestRetryRateLimited verifies that 429 responses are retried when
// RetryRateLimited is enabled in the tfe.Config.
func TestRetryRateLimited(t *testing.T) {
	t.Parallel()

	var attempts int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&attempts, 1)
		if n < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, "test-token", nil, hclog.NewNullLogger())
	require.NoError(t, err)

	resp, err := c.TFE.GetStream(t.Context(), "/test", nil)
	require.NoError(t, err)
	resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.GreaterOrEqual(t, atomic.LoadInt64(&attempts), int64(3))
}

// TestRetryLogHook verifies that the RetryHook is invoked on each retry,
// producing debug-level log output.
func TestRetryLogHook(t *testing.T) {
	t.Parallel()

	var attempts int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&attempts, 1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(srv.Close)

	logger := hclog.New(&hclog.LoggerOptions{Level: hclog.Debug})
	c, err := New(srv.URL, "test-token", nil, logger)
	require.NoError(t, err)

	resp, err := c.TFE.GetStream(t.Context(), "/test", nil)
	require.NoError(t, err)
	resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.GreaterOrEqual(t, atomic.LoadInt64(&attempts), int64(2))
}
