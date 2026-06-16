// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package ratelimit provides per-client rate limiting for the OTLP proxy.
//
// The RateLimiter interface abstracts the backend so the in-memory limiter
// used in v1 can later be swapped for a distributed backend (e.g. Redis)
// without touching the request pipeline. Callers must fail open: telemetry is
// non-critical, so a limiter error should never reject a request.
package ratelimit

// RateLimiter decides whether a request identified by key may proceed.
//
// Implementations should be safe for concurrent use. The error return exists
// for future remote backends; callers must treat any error as "allow" (fail
// open) and must never surface it to clients.
type RateLimiter interface {
	// Allow reports whether a request from key is permitted right now.
	Allow(key string) (bool, error)
}
