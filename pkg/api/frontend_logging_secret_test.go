package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/api/frontendlogging"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/grafana/grafana/pkg/web"
)

func TestFrontendLoggingOmitsRequestAuthorizationAndSession(t *testing.T) {
	const (
		bearerCanary  = "nds10-bearer-canary"
		sessionCanary = "nds10-session-canary"
	)

	var recorded strings.Builder
	capture := log.LoggerFunc(func(keyvals ...any) error {
		for _, kv := range keyvals {
			recorded.WriteString(stringify(kv))
			recorded.WriteByte('\n')
		}
		return nil
	})
	original := frontendLogger.GetLogger()
	frontendLogger.Swap(level.NewFilter(capture, level.AllowAll()))
	t.Cleanup(func() {
		frontendLogger.Swap(original)
	})

	ts := time.Date(2020, 10, 22, 6, 29, 29, 0, time.UTC)
	event := frontendlogging.FrontendGrafanaJavascriptAgentEvent{
		Meta: frontendlogging.Meta{
			Page: frontendlogging.Page{URL: "http://localhost:3000/d/abc"},
			User: frontendlogging.User{ID: "45", Email: "dev@example.com"},
		},
		Logs: []frontendlogging.Log{{
			Message:   "panel query failed",
			LogLevel:  frontendlogging.LogLevelInfo,
			Timestamp: ts,
			Context:   map[string]string{"dashboardUid": "abc"},
		}},
		Exceptions: []frontendlogging.Exception{{
			Type:      "Error",
			Value:     "panel query failed",
			Timestamp: ts,
			Stacktrace: &frontendlogging.Stacktrace{Frames: []frontendlogging.Frame{{
				Function: "loadDashboard",
				Filename: "app.js",
				Lineno:   10,
				Colno:    2,
			}}},
		}},
	}

	cfg := setting.NewCfg()
	store := frontendlogging.NewSourceMapStore(cfg, &fakePluginStaticRouteResolver{}, func(string, string) ([]byte, error) {
		return nil, os.ErrNotExist
	})
	handler := GrafanaJavascriptAgentLogMessageHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/log-grafana-javascript-agent", mockRequestBody(event))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerCanary)
	req.Header.Set("Cookie", "grafana_session="+sessionCanary)
	rec := httptest.NewRecorder()
	handler(nil, &web.Context{Req: req, Resp: web.Rw(rec, req)})

	require.Equal(t, http.StatusAccepted, rec.Code)
	out := recorded.String()
	require.Contains(t, out, "panel query failed")
	require.Contains(t, out, "abc")
	require.Contains(t, out, "loadDashboard")
	require.Contains(t, out, "http://localhost:3000/d/abc")
	require.NotContains(t, out, bearerCanary)
	require.NotContains(t, out, sessionCanary)
	require.NotContains(t, out, "Bearer")
	require.NotContains(t, out, "grafana_session")
	require.NotContains(t, out, "Authorization")
}

func stringify(v any) string {
	return strings.ReplaceAll(fmt.Sprint(v), "\n", " ")
}
