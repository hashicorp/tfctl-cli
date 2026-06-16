// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package clientip resolves the originating client IP for a request reaching
// a public endpoint behind a known number of trusted reverse proxies.
//
// X-Forwarded-For is attacker-controlled: a client may inject arbitrary
// leftmost entries. Only the entry appended by the closest trusted hop can be
// believed, so the resolver counts TrustedHopCount positions from the right
// and ignores everything to its left. X-Real-IP is never consulted.
package clientip

import (
	"net"
	"net/http"
	"strings"
)

// Resolver extracts the client IP using a fixed number of trusted proxy hops.
type Resolver struct {
	// TrustedHopCount is the number of trusted reverse proxies/load balancers
	// in front of the service. The client IP is taken this many positions from
	// the right end of X-Forwarded-For. Zero means trust no proxies and always
	// use RemoteAddr.
	TrustedHopCount int
}

// ClientIP returns the resolved client IP as a string. It prefers the
// trusted X-Forwarded-For entry and falls back to RemoteAddr (port stripped)
// when the header is absent, too short, or contains an invalid address.
func (r Resolver) ClientIP(req *http.Request) string {
	if r.TrustedHopCount > 0 {
		if ip, ok := r.fromForwardedFor(req); ok {
			return ip
		}
	}
	return remoteAddrIP(req.RemoteAddr)
}

// fromForwardedFor returns the trusted X-Forwarded-For entry, if present and
// valid. The boolean reports whether a usable address was found.
func (r Resolver) fromForwardedFor(req *http.Request) (string, bool) {
	raw := req.Header.Get("X-Forwarded-For")
	if raw == "" {
		return "", false
	}

	parts := strings.Split(raw, ",")
	entries := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			entries = append(entries, trimmed)
		}
	}

	idx := len(entries) - r.TrustedHopCount
	if idx < 0 || idx >= len(entries) {
		return "", false
	}

	candidate := entries[idx]
	if net.ParseIP(candidate) == nil {
		return "", false
	}
	return candidate, true
}

// remoteAddrIP strips the port from a RemoteAddr, returning the value verbatim
// if it has no port.
func remoteAddrIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}
