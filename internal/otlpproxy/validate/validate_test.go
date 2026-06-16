// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// marshalRequest builds a serialized ExportTraceServiceRequest. A non-empty
// serviceName adds a single resource span carrying that service.name; an empty
// serviceName produces a request with one resource span but no service.name
// attribute.
func marshalRequest(t *testing.T, serviceName string) []byte {
	t.Helper()

	res := &resourcepb.Resource{}
	if serviceName != "" {
		res.Attributes = []*commonpb.KeyValue{
			{
				Key:   "service.name",
				Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: serviceName}},
			},
		}
	}

	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{Resource: res}},
	}
	b, err := proto.Marshal(req)
	require.NoError(t, err)
	return b
}

func TestPayload_ValidMatchingServiceName(t *testing.T) {
	t.Parallel()

	body := marshalRequest(t, "tfctl")
	assert.NoError(t, Payload(body, "", "tfctl"))
}

func TestPayload_MismatchedServiceNameRejected(t *testing.T) {
	t.Parallel()

	body := marshalRequest(t, "evil-service")
	assert.Error(t, Payload(body, "", "tfctl"))
}

func TestPayload_NoServiceNameAllowedWhenExpected(t *testing.T) {
	t.Parallel()

	// A decodable request without a service.name attribute is accepted; v1
	// validation gates obvious mismatches, not attribute presence.
	body := marshalRequest(t, "")
	assert.NoError(t, Payload(body, "", "tfctl"))
}

func TestPayload_EmptyExpectedSkipsServiceNameCheck(t *testing.T) {
	t.Parallel()

	body := marshalRequest(t, "anything")
	assert.NoError(t, Payload(body, "", ""))
}

func TestPayload_UndecodableRejected(t *testing.T) {
	t.Parallel()

	// Field 1 (resource_spans) declared as a 5-byte length-delimited value but
	// truncated: a clean protobuf decode failure.
	bad := []byte{0x0a, 0x05, 0x01, 0x02}
	assert.Error(t, Payload(bad, "", "tfctl"))
}

func TestPayload_GzipEncodingSkipsDecode(t *testing.T) {
	t.Parallel()

	// Bytes that would not decode are accepted unchecked when gzip-encoded,
	// because v1 does not decompress.
	bad := []byte{0x0a, 0x05, 0x01, 0x02}
	assert.NoError(t, Payload(bad, "gzip", "tfctl"))
}
