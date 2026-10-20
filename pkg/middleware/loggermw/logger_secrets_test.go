package loggermw

import (
	"bytes"
	"net/http"
	"testing"

	gokitlog "github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/infra/log/text"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/services/featuremgmt"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/grafana/grafana/pkg/web"
)

// #nosec G101 -- test fixtures, not real credentials
const (
	bearerSecret  = "test-secret-bearer-1f2e3d"
	cookieSecret  = "test-secret-session-4c5b6a"
	apiKeySecret  = "test-secret-apikey-7d8e9f"
	querySecret   = "test-secret-query-0a1b2c"
	refererSecret = "test-secret-referer-3d4e5f"
)

func TestPrepareLogParams_DoesNotLeakCredentials(t *testing.T) {
	req := mustRequest(http.NewRequest(http.MethodGet, "/api/dashboards/uid/abc?auth_token="+querySecret, nil))
	req.Header.Set("Authorization", "Bearer "+bearerSecret)
	req.Header.Set("Cookie", "grafana_session="+cookieSecret)
	req.Header.Set("X-Api-Key", apiKeySecret)
	req.Header.Set("Referer", "https://grafana.example.com/d/abc?auth_token="+refererSecret+"&orgId=1")

	cfg := setting.NewCfg()
	cfg.RouterLogging = true
	service, ok := Provide(cfg, featuremgmt.WithFeatures()).(*loggerImpl)
	require.True(t, ok)

	c := &contextmodel.ReqContext{
		Context: &web.Context{
			Req:  req,
			Resp: mockResponseWriter{status: http.StatusOK},
		},
	}
	params, _ := service.prepareLogParams(c, 0)

	formats := map[string]func(*bytes.Buffer) gokitlog.Logger{
		"text": func(b *bytes.Buffer) gokitlog.Logger { return text.NewTextLogger(b) },
		"json": func(b *bytes.Buffer) gokitlog.Logger { return gokitlog.NewJSONLogger(b) },
	}
	for name, newLogger := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			kv := append([]any{"msg", "Request Completed"}, params...)
			require.NoError(t, newLogger(&buf).Log(kv...))

			out := buf.String()
			require.Contains(t, out, "/api/dashboards/uid/abc", "sanity check: the request path should be logged")
			for _, secret := range []string{bearerSecret, cookieSecret, apiKeySecret, querySecret, refererSecret} {
				assert.NotContains(t, out, secret)
			}
			assert.NotContains(t, out, "Bearer")
			assert.Contains(t, out, "auth_token=hidden", "referer auth_token should be masked, not dropped")
		})
	}
}
