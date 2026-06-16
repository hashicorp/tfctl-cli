// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package forward

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HTTP and NoOp must satisfy the Forwarder interface.
var (
	_ Forwarder = (*HTTP)(nil)
	_ Forwarder = (*NoOp)(nil)
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// captured records what a fake upstream received.
type captured struct {
	mu              sync.Mutex
	hits            int
	method          string
	path            string
	body            []byte
	contentType     string
	contentEncoding string
}

func (c *captured) record(r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits++
	c.method = r.Method
	c.path = r.URL.Path
	c.contentType = r.Header.Get("Content-Type")
	c.contentEncoding = r.Header.Get("Content-Encoding")
	c.body, _ = io.ReadAll(r.Body)
}

func TestHTTP_ForwardsExactBytesAndHeaders(t *testing.T) {
	t.Parallel()

	cap := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.record(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	body := []byte{0x0a, 0x00, 0xff, 0x10, 0x2a} // Arbitrary protobuf-ish bytes.
	f := NewHTTP(srv.URL, time.Second, discardLogger())

	err := f.Forward(context.Background(), body, "application/x-protobuf", "")
	require.NoError(t, err)

	cap.mu.Lock()
	defer cap.mu.Unlock()
	assert.Equal(t, 1, cap.hits)
	assert.Equal(t, http.MethodPost, cap.method)
	assert.Equal(t, "/v1/traces", cap.path)
	assert.Equal(t, body, cap.body, "upstream must receive the exact original bytes")
	assert.Equal(t, "application/x-protobuf", cap.contentType)
	assert.Empty(t, cap.contentEncoding, "no Content-Encoding set when none provided")
}

func TestHTTP_PreservesContentEncoding(t *testing.T) {
	t.Parallel()

	cap := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.record(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := NewHTTP(srv.URL, time.Second, discardLogger())
	err := f.Forward(context.Background(), []byte("gzipped"), "application/x-protobuf", "gzip")
	require.NoError(t, err)

	cap.mu.Lock()
	defer cap.mu.Unlock()
	assert.Equal(t, "gzip", cap.contentEncoding)
}

func TestHTTP_TrailingSlashEndpoint(t *testing.T) {
	t.Parallel()

	cap := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.record(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Endpoint with a trailing slash must not produce a doubled slash.
	f := NewHTTP(srv.URL+"/", time.Second, discardLogger())
	require.NoError(t, f.Forward(context.Background(), []byte("x"), "application/x-protobuf", ""))

	cap.mu.Lock()
	defer cap.mu.Unlock()
	assert.Equal(t, "/v1/traces", cap.path)
}

func TestHTTP_UpstreamErrorStatusReturnsError(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))

		f := NewHTTP(srv.URL, time.Second, discardLogger())
		err := f.Forward(context.Background(), []byte("x"), "application/x-protobuf", "")
		assert.Error(t, err, "status %d should be an error", status)

		srv.Close()
	}
}

func TestHTTP_UpstreamUnreachableReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := srv.URL
	srv.Close() // Now nothing is listening.

	f := NewHTTP(url, time.Second, discardLogger())
	err := f.Forward(context.Background(), []byte("x"), "application/x-protobuf", "")
	assert.Error(t, err)
}

func TestHTTP_UpstreamTimeoutReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := NewHTTP(srv.URL, 10*time.Millisecond, discardLogger())
	err := f.Forward(context.Background(), []byte("x"), "application/x-protobuf", "")
	assert.Error(t, err)
}

func TestNoOp_ReturnsNil(t *testing.T) {
	t.Parallel()

	f := NewNoOp(discardLogger())
	err := f.Forward(context.Background(), []byte("x"), "application/x-protobuf", "")
	assert.NoError(t, err)
}
