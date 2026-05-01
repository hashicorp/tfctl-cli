// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package openapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchema_OperationById(t *testing.T) {
	embedded, err := SchemaFactory(nil)
	require.NoError(t, err)

	op, err := embedded.OperationByID("getWorkspace")
	require.NoError(t, err)

	require.NotNil(t, op)
	require.Equal(t, "getWorkspace", op.OperationID)

	nf, err := embedded.OperationByID("foobar")
	require.ErrorContains(t, err, "operation with ID \"foobar\" not found")
	require.Nil(t, nf)
}
