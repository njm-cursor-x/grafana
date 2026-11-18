package featuremgmt

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/infra/log"
)

// The log package reads this toggle from config before feature management starts.
func TestSlogLoggingCoreToggleName(t *testing.T) {
	require.Equal(t, FlagGrafanaSlogLoggingCore, log.SlogCoreFeatureToggle)
}
