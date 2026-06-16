// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package clientip

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolver_ClientIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hopCount   int
		remoteAddr string
		xff        string // raw X-Forwarded-For header; empty means absent.
		want       string
	}{
		{
			name:       "no XFF falls back to RemoteAddr without port",
			hopCount:   1,
			remoteAddr: "203.0.113.7:54321",
			want:       "203.0.113.7",
		},
		{
			name:       "single trusted hop honors rightmost entry",
			hopCount:   1,
			remoteAddr: "10.0.0.1:1234",
			xff:        "198.51.100.23",
			want:       "198.51.100.23",
		},
		{
			name:       "spoofed leftmost XFF entry is ignored",
			hopCount:   1,
			remoteAddr: "10.0.0.1:1234",
			// Attacker prepends a fake IP; our LB appends the real client last.
			xff:  "1.2.3.4, 198.51.100.23",
			want: "198.51.100.23",
		},
		{
			name:       "two trusted hops selects entry two from the right",
			hopCount:   2,
			remoteAddr: "10.0.0.1:1234",
			xff:        "198.51.100.23, 10.0.0.9",
			want:       "198.51.100.23",
		},
		{
			name:       "deep spoofing with two trusted hops still ignored",
			hopCount:   2,
			remoteAddr: "10.0.0.1:1234",
			xff:        "9.9.9.9, 8.8.8.8, 198.51.100.23, 10.0.0.9",
			want:       "198.51.100.23",
		},
		{
			name:       "fewer XFF entries than hop count falls back to RemoteAddr",
			hopCount:   2,
			remoteAddr: "203.0.113.7:443",
			xff:        "198.51.100.23",
			want:       "203.0.113.7",
		},
		{
			name:       "hop count zero ignores XFF entirely",
			hopCount:   0,
			remoteAddr: "203.0.113.7:443",
			xff:        "198.51.100.23",
			want:       "203.0.113.7",
		},
		{
			name:       "invalid selected entry falls back to RemoteAddr",
			hopCount:   1,
			remoteAddr: "203.0.113.7:443",
			xff:        "not-an-ip",
			want:       "203.0.113.7",
		},
		{
			name:       "whitespace around entries is trimmed",
			hopCount:   1,
			remoteAddr: "10.0.0.1:1234",
			xff:        "  1.2.3.4 ,  198.51.100.23  ",
			want:       "198.51.100.23",
		},
		{
			name:       "IPv6 RemoteAddr without XFF",
			hopCount:   1,
			remoteAddr: "[2001:db8::1]:9999",
			want:       "2001:db8::1",
		},
		{
			name:       "RemoteAddr without port used verbatim",
			hopCount:   1,
			remoteAddr: "203.0.113.7",
			want:       "203.0.113.7",
		},
		{
			name:       "empty XFF entries are skipped",
			hopCount:   1,
			remoteAddr: "10.0.0.1:1234",
			xff:        "198.51.100.23, ,",
			want:       "198.51.100.23",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &http.Request{
				Header:     http.Header{},
				RemoteAddr: tt.remoteAddr,
			}
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}

			r := Resolver{TrustedHopCount: tt.hopCount}
			assert.Equal(t, tt.want, r.ClientIP(req))
		})
	}
}

// TestResolver_XRealIPIgnored proves a spoofed X-Real-IP cannot influence the
// resolved client IP.
func TestResolver_XRealIPIgnored(t *testing.T) {
	t.Parallel()

	req := &http.Request{
		Header:     http.Header{},
		RemoteAddr: "10.0.0.1:1234",
	}
	req.Header.Set("X-Real-IP", "6.6.6.6")
	req.Header.Set("X-Forwarded-For", "198.51.100.23")

	r := Resolver{TrustedHopCount: 1}
	assert.Equal(t, "198.51.100.23", r.ClientIP(req))
}
