//go:build !arm

package sql

import (
	"testing"

	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/require"
)

func TestConvertDataType(t *testing.T) {
	t.Parallel()

	require.Equal(t, types.Int64, convertDataType(data.FieldTypeInt64))
	require.Equal(t, types.Text, convertDataType(data.FieldTypeString))
	require.Equal(t, types.Boolean, convertDataType(data.FieldTypeBool))
	require.Equal(t, types.JSON, convertDataType(data.FieldTypeJSON))
	require.Equal(t, types.JSON, convertDataType(data.FieldType(255)))
}
