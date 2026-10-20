//go:build !arm

package sql

import (
	"testing"

	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/infra/log/logtest"
)

func TestConvertDataType_UnsupportedTypeLogsStructuredWarning(t *testing.T) {
	fake := &logtest.Fake{}
	orig := frameTableLogger
	frameTableLogger = fake
	t.Cleanup(func() { frameTableLogger = orig })

	require.Equal(t, types.JSON, convertDataType(data.FieldTypeUnknown))

	require.Equal(t, 1, fake.WarnLogs.Calls)
	require.Equal(t, "unsupported field type, falling back to JSON", fake.WarnLogs.Message)
	require.Equal(t, []any{"fieldType", data.FieldTypeUnknown.String()}, fake.WarnLogs.Ctx)
}

func TestConvertDataType_SupportedTypeDoesNotLog(t *testing.T) {
	fake := &logtest.Fake{}
	orig := frameTableLogger
	frameTableLogger = fake
	t.Cleanup(func() { frameTableLogger = orig })

	require.Equal(t, types.Text, convertDataType(data.FieldTypeString))
	require.Zero(t, fake.WarnLogs.Calls)
}
