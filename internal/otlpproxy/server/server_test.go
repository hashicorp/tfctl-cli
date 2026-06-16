// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/clientip"
	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/forward"
	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/ratelimit"
)

const protobufContentType = "application/x-protobuf"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeLimiter is a controllable RateLimiter for tests.
type fakeLimiter struct {
	allow bool
	err   error

	mu   sync.Mutex
	keys []string
}

func (f *fakeLimiter) Allow(key string) (bool, error) {
	f.mu.Lock()
	f.keys = append(f.keys, key)
	f.mu.Unlock()
	return f.allow, f.err
}

func (f *fakeLimiter) seenKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.keys...)
}

// fakeForwarder records what it received and returns a configurable error.
type fakeForwarder struct {
	err error

	mu       sync.Mutex
	calls    int
	lastBody []byte
	lastCT   string
	lastCE   string
}

func (f *fakeForwarder) Forward(_ context.Context, body []byte, contentType, contentEncoding string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastBody = append([]byte(nil), body...)
	f.lastCT = contentType
	f.lastCE = contentEncoding
	return f.err
}

func (f *fakeForwarder) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// baseOptions returns Options wired with permissive fakes that tests override.
func baseOptions() Options {
	return Options{
		Limiter:                    &fakeLimiter{allow: true},
		Forwarder:                  &fakeForwarder{},
		IPResolver:                 clientip.Resolver{TrustedHopCount: 1},
		Logger:                     discardLogger(),
		MaxBodyBytes:               1 << 20,
		RequireProtobufContentType: true,
		ValidatePayload:            false,
		ExpectedServiceName:        "tfctl",
	}
}

// tracesRequest builds a POST /v1/traces request with protobuf content type.
func tracesRequest(body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", protobufContentType)
	req.RemoteAddr = "10.0.0.1:1234"
	return req
}

// marshalTrace builds a serialized ExportTraceServiceRequest for serviceName.
func marshalTrace(t *testing.T, serviceName string) []byte {
	t.Helper()
	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{
							Key:   "service.name",
							Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: serviceName}},
						},
					},
				},
			},
		},
	}
	b, err := proto.Marshal(req)
	require.NoError(t, err)
	return b
}

func TestRouting_WrongPathReturns404(t *testing.T) {
	t.Parallel()

	h := New(baseOptions())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRouting_WrongMethodReturns405(t *testing.T) {
	t.Parallel()

	h := New(baseOptions())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/traces", nil))

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHealth_ReturnsOKWithoutForwarding(t *testing.T) {
	t.Parallel()

	fwd := &fakeForwarder{}
	opts := baseOptions()
	opts.Forwarder = fwd

	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Body.String())
	assert.Equal(t, 0, fwd.callCount(), "health must not call upstream")
}

func TestTraces_SuccessReturnsEmpty200AndForwards(t *testing.T) {
	t.Parallel()

	fwd := &fakeForwarder{}
	opts := baseOptions()
	opts.Forwarder = fwd

	body := []byte{0x0a, 0x00, 0x01, 0x02}
	req := tracesRequest(body)
	req.Header.Set("Content-Encoding", "gzip")

	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String(), "success response body must be empty")
	assert.Equal(t, 1, fwd.callCount())
	assert.Equal(t, body, fwd.lastBody, "forwarder receives exact original bytes")
	assert.Equal(t, protobufContentType, fwd.lastCT)
	assert.Equal(t, "gzip", fwd.lastCE, "Content-Encoding preserved")
}

func TestTraces_ForwardingToRealUpstream(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		hits     int
		gotBody  []byte
		gotCT    string
		gotPath  string
		gotMeth  string
		gotCEHdr string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		hits++
		gotMeth = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotCEHdr = r.Header.Get("Content-Encoding")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	opts := baseOptions()
	opts.Forwarder = forward.NewHTTP(upstream.URL, time.Second, discardLogger())

	body := []byte{0x0a, 0x00, 0xff, 0x2a}
	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, tracesRequest(body))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, hits, "upstream called exactly once")
	assert.Equal(t, http.MethodPost, gotMeth)
	assert.Equal(t, "/v1/traces", gotPath)
	assert.Equal(t, body, gotBody, "upstream receives exact original bytes")
	assert.Equal(t, protobufContentType, gotCT)
	assert.Empty(t, gotCEHdr)
}

func TestTraces_UpstreamErrorReturns503(t *testing.T) {
	t.Parallel()

	opts := baseOptions()
	opts.Forwarder = &fakeForwarder{err: errors.New("upstream unreachable")}

	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, tracesRequest([]byte("x")))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.NotContains(t, rec.Body.String(), "upstream unreachable", "upstream error must not leak to client")
}

func TestTraces_NoOpModeReturns200WithoutUpstreamCall(t *testing.T) {
	t.Parallel()

	// A spy upstream that must never be contacted in no-op mode.
	var hits int
	var mu sync.Mutex
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	opts := baseOptions()
	opts.Forwarder = forward.NewNoOp(discardLogger()) // No-op: no upstream wired.

	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, tracesRequest([]byte("x")))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, hits, "no-op mode makes no upstream call")
}

func TestTraces_BodyOverMaxReturns413(t *testing.T) {
	t.Parallel()

	opts := baseOptions()
	opts.MaxBodyBytes = 16
	fwd := &fakeForwarder{}
	opts.Forwarder = fwd

	body := bytes.Repeat([]byte{0x01}, 1024) // Well over the cap.
	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, tracesRequest(body))

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, 0, fwd.callCount(), "oversized body must not be forwarded")
}

func TestTraces_LyingSmallContentLengthStillCapped(t *testing.T) {
	t.Parallel()

	opts := baseOptions()
	opts.MaxBodyBytes = 16
	fwd := &fakeForwarder{}
	opts.Forwarder = fwd

	body := bytes.Repeat([]byte{0x01}, 1024)
	req := tracesRequest(body)
	// Spoof a small Content-Length: the MaxBytesReader must still cap the read.
	req.ContentLength = 5

	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, 0, fwd.callCount())
}

func TestTraces_HonestLargeContentLengthRejectedEarly(t *testing.T) {
	t.Parallel()

	opts := baseOptions()
	opts.MaxBodyBytes = 16

	body := bytes.Repeat([]byte{0x01}, 1024)
	req := tracesRequest(body) // ContentLength == 1024 from the reader.

	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestTraces_ContentTypeEnforcement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		enforce     bool
		contentType string
		wantStatus  int
	}{
		{"enforced wrong type rejected", true, "application/json", http.StatusUnsupportedMediaType},
		{"enforced empty type rejected", true, "", http.StatusUnsupportedMediaType},
		{"enforced correct type accepted", true, protobufContentType, http.StatusOK},
		{"enforced correct type with params accepted", true, "application/x-protobuf; charset=utf-8", http.StatusOK},
		{"not enforced wrong type accepted", false, "application/json", http.StatusOK},
		{"not enforced empty type accepted", false, "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := baseOptions()
			opts.RequireProtobufContentType = tt.enforce

			req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("x")))
			req.RemoteAddr = "10.0.0.1:1234"
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			h := New(opts)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestTraces_RateLimitedReturns429(t *testing.T) {
	t.Parallel()

	opts := baseOptions()
	opts.Limiter = &fakeLimiter{allow: false}
	fwd := &fakeForwarder{}
	opts.Forwarder = fwd

	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, tracesRequest([]byte("x")))

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, 0, fwd.callCount(), "rate-limited request must not be forwarded")
}

func TestTraces_RateLimitPerIPWithRealLimiter(t *testing.T) {
	t.Parallel()

	// Burst of 1 so the second request from the same IP is limited while a
	// different IP remains unaffected.
	clk := time.Unix(1_700_000_000, 0)
	opts := baseOptions()
	opts.Limiter = ratelimit.NewMemory(ratelimit.MemoryConfig{
		PerMinute:  60,
		Burst:      1,
		MaxEntries: 100,
		TTL:        time.Hour,
		Now:        func() time.Time { return clk },
	})

	h := New(opts)

	send := func(remoteAddr string) int {
		req := tracesRequest([]byte("x"))
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	assert.Equal(t, http.StatusOK, send("203.0.113.1:1111"), "first request from IP A allowed")
	assert.Equal(t, http.StatusTooManyRequests, send("203.0.113.1:2222"), "second request from IP A limited")
	assert.Equal(t, http.StatusOK, send("203.0.113.2:3333"), "different IP B unaffected")
}

func TestTraces_SpoofedXFFDoesNotEvadeLimiter(t *testing.T) {
	t.Parallel()

	clk := time.Unix(1_700_000_000, 0)
	opts := baseOptions()
	opts.IPResolver = clientip.Resolver{TrustedHopCount: 1}
	opts.Limiter = ratelimit.NewMemory(ratelimit.MemoryConfig{
		PerMinute:  60,
		Burst:      1,
		MaxEntries: 100,
		TTL:        time.Hour,
		Now:        func() time.Time { return clk },
	})

	h := New(opts)

	send := func(xff string) int {
		req := tracesRequest([]byte("x"))
		req.RemoteAddr = "10.0.0.1:1234"
		req.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// Same real client (rightmost) behind two different spoofed leftmost
	// entries: both must hit the same bucket, so the second is limited.
	assert.Equal(t, http.StatusOK, send("1.1.1.1, 9.9.9.9"))
	assert.Equal(t, http.StatusTooManyRequests, send("2.2.2.2, 9.9.9.9"))
}

func TestTraces_LimiterErrorFailsOpen(t *testing.T) {
	t.Parallel()

	opts := baseOptions()
	opts.Limiter = &fakeLimiter{allow: false, err: errors.New("backend down")}
	fwd := &fakeForwarder{}
	opts.Forwarder = fwd

	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, tracesRequest([]byte("x")))

	assert.Equal(t, http.StatusOK, rec.Code, "limiter error must fail open and allow the request")
	assert.Equal(t, 1, fwd.callCount(), "request forwarded despite limiter error")
}

func TestTraces_ResolvedIPUsedAsLimiterKey(t *testing.T) {
	t.Parallel()

	lim := &fakeLimiter{allow: true}
	opts := baseOptions()
	opts.Limiter = lim
	opts.IPResolver = clientip.Resolver{TrustedHopCount: 1}

	req := tracesRequest([]byte("x"))
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.23")

	h := New(opts)
	h.ServeHTTP(httptest.NewRecorder(), req)

	keys := lim.seenKeys()
	require.Len(t, keys, 1)
	assert.Equal(t, "198.51.100.23", keys[0], "limiter keyed on the trusted client IP, not the spoofed entry")
}

func TestTraces_ValidationEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       []byte
		expectName string
		wantStatus int
		wantFwd    bool
	}{
		{
			name:       "valid matching service name forwarded",
			body:       marshalTrace(t, "tfctl"),
			expectName: "tfctl",
			wantStatus: http.StatusOK,
			wantFwd:    true,
		},
		{
			name:       "mismatched service name rejected",
			body:       marshalTrace(t, "evil"),
			expectName: "tfctl",
			wantStatus: http.StatusBadRequest,
			wantFwd:    false,
		},
		{
			name:       "undecodable body rejected",
			body:       []byte{0x0a, 0x05, 0x01, 0x02},
			expectName: "tfctl",
			wantStatus: http.StatusBadRequest,
			wantFwd:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fwd := &fakeForwarder{}
			opts := baseOptions()
			opts.ValidatePayload = true
			opts.ExpectedServiceName = tt.expectName
			opts.Forwarder = fwd

			h := New(opts)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, tracesRequest(tt.body))

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantFwd {
				assert.Equal(t, 1, fwd.callCount())
				assert.Equal(t, tt.body, fwd.lastBody, "original bytes forwarded unchanged")
			} else {
				assert.Equal(t, 0, fwd.callCount())
			}
		})
	}
}

func TestTraces_ValidationDisabledForwardsBlindly(t *testing.T) {
	t.Parallel()

	fwd := &fakeForwarder{}
	opts := baseOptions()
	opts.ValidatePayload = false
	opts.Forwarder = fwd

	// Undecodable bytes are forwarded as-is when validation is off.
	body := []byte{0x0a, 0x05, 0x01, 0x02}
	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, tracesRequest(body))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, fwd.callCount())
	assert.Equal(t, body, fwd.lastBody)
}

func TestTraces_PipelineOrderRateLimitBeforeContentType(t *testing.T) {
	t.Parallel()

	// A rate-limited request with a wrong content type must return 429, proving
	// rate limiting runs before the content-type check.
	opts := baseOptions()
	opts.Limiter = &fakeLimiter{allow: false}
	opts.RequireProtobufContentType = true

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:1234"

	h := New(opts)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}
