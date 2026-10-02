package loggermw

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/apimachinery/errutil"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/services/featuremgmt"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/grafana/grafana/pkg/web"
)

func TestPrepareLogParamsOmitsAuthorizationAndSession(t *testing.T) {
	const (
		bearerCanary   = "nds10-bearer-canary"
		sessionCanary  = "nds10-session-canary"
		queryCanary    = "nds10-query-canary"
		refererCanary  = "nds10-referer-canary"
		userinfoCanary = "nds10-userinfo-canary"
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"https://user:"+userinfoCanary+"@grafana.example/api/search?api_key="+queryCanary,
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+bearerCanary)
	req.Header.Set("Cookie", "grafana_session="+sessionCanary)
	req.Header.Set("Referer", "https://grafana.example/d/abc?auth_token="+refererCanary+"&q=ok")

	cfg := setting.NewCfg()
	cfg.RouterLogging = true
	service, ok := Provide(cfg, featuremgmt.WithFeatures()).(*loggerImpl)
	require.True(t, ok)

	c := &contextmodel.ReqContext{
		Context: &web.Context{
			Req:  req,
			Resp: mockResponseWriter{status: http.StatusOK},
		},
		Error: fmt.Errorf("upstream status %%s"),
	}

	params, level := service.prepareLogParams(c, 0)
	text := stringifyLogParams(params)

	require.Equal(t, errutil.LevelInfo, level)
	require.Equal(t, "GET", paramValue(t, params, "method"))
	require.Equal(t, "/api/search", paramValue(t, params, "path"))
	require.Equal(t, http.StatusOK, paramValue(t, params, "status"))
	require.Equal(t, "upstream status %s", paramValue(t, params, "error"))
	require.Contains(t, paramValue(t, params, "referer"), "auth_token=hidden")
	require.NotContains(t, text, bearerCanary)
	require.NotContains(t, text, sessionCanary)
	require.NotContains(t, text, queryCanary)
	require.NotContains(t, text, refererCanary)
	require.NotContains(t, text, userinfoCanary)
	require.NotContains(t, text, "Bearer")
	require.NotContains(t, text, "grafana_session")
	require.NotContains(t, text, "Authorization")
}

func stringifyLogParams(params []any) string {
	var b strings.Builder
	for _, p := range params {
		fmt.Fprintf(&b, "%v\n", p)
	}
	return b.String()
}

func paramValue(t *testing.T, params []any, key string) any {
	t.Helper()
	require.Zero(t, len(params)%2)
	for i := 0; i < len(params); i += 2 {
		got, ok := params[i].(string)
		if ok && got == key {
			return params[i+1]
		}
	}
	t.Fatalf("missing log param %q", key)
	return nil
}
