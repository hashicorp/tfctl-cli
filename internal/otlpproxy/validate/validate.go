// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package validate provides optional, best-effort decoding of OTLP trace
// export bodies. It is disabled by default: deeper schema enforcement is the
// upstream collector's job. When enabled, the proxy decodes a copy of the body
// only to gate obviously bad payloads and always forwards the original bytes
// unchanged.
package validate

import (
	"fmt"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

// serviceNameKey is the OTLP resource attribute identifying the emitting
// service.
const serviceNameKey = "service.name"

// Payload decodes body as an OTLP ExportTraceServiceRequest and applies light
// validation. It never mutates or returns the payload; callers forward the
// original bytes regardless of the result.
//
// Rules:
//   - A gzip Content-Encoding skips decoding entirely (v1 does not decompress).
//   - An undecodable body is rejected.
//   - When expectedServiceName is non-empty, a resource whose service.name is
//     present but differs is rejected. A missing service.name is accepted.
func Payload(body []byte, contentEncoding, expectedServiceName string) error {
	if contentEncoding == "gzip" {
		return nil
	}

	var req coltracepb.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("decode OTLP trace request: %w", err)
	}

	if expectedServiceName == "" {
		return nil
	}

	for _, rs := range req.GetResourceSpans() {
		for _, attr := range rs.GetResource().GetAttributes() {
			if attr.GetKey() != serviceNameKey {
				continue
			}
			if got := attr.GetValue().GetStringValue(); got != expectedServiceName {
				return fmt.Errorf("unexpected service.name %q (want %q)", got, expectedServiceName)
			}
		}
	}

	return nil
}
