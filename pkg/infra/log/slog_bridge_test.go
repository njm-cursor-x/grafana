package log

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/infra/log/slogadapter"
)

func TestSlogHandler_ForwardsLevelsGroupsAndRedacts(t *testing.T) {
	scenario := newLoggerScenario(t)
	h := slogadapter.New(New("slog"))

	require.True(t, h.Enabled(context.Background(), slog.LevelDebug))
	require.Same(t, h, h.WithGroup(""))
	require.Same(t, h, h.WithAttrs(nil))

	info := slog.NewRecord(time.Time{}, slog.LevelInfo, "started", 0)
	info.AddAttrs(slog.Int("orgId", 7))
	require.NoError(t, h.Handle(context.Background(), info))

	require.NoError(t, h.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelDebug, "trace", 0)))
	require.NoError(t, h.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelWarn, "slow", 0)))

	errRec := slog.NewRecord(time.Time{}, slog.LevelError, "upstream rejected Authorization: Bearer test-secret", 0)
	errRec.AddAttrs(
		slog.String("Authorization", "Bearer test-secret"),
		slog.String("dashboardTitle", "Sales %s"),
		slog.Group("nested",
			slog.String("token", "test-secret"),
			slog.String("name", "ok"),
			slog.Group("inner", slog.String("password", "test-secret")),
		),
	)
	require.NoError(t, h.Handle(context.Background(), errRec))

	grouped := h.WithGroup("req").WithGroup("http").WithAttrs([]slog.Attr{slog.String("id", "abc")})
	child := slog.NewRecord(time.Time{}, slog.LevelInfo, "child", 0)
	child.AddAttrs(slog.String("method", "GET"))
	require.NoError(t, grouped.Handle(context.Background(), child))

	started := lineWithMsg(t, scenario.loggedArgs, "started")
	require.Equal(t, "info", levelOf(started))
	orgID, ok := rawVal(started, "orgId")
	require.True(t, ok)
	require.Equal(t, int64(7), orgID)

	require.Equal(t, "debug", levelOf(lineWithMsg(t, scenario.loggedArgs, "trace")))
	require.Equal(t, "warn", levelOf(lineWithMsg(t, scenario.loggedArgs, "slow")))

	failedLine := lineWithMsgPrefix(t, scenario.loggedArgs, "upstream rejected")
	require.Equal(t, "eror", levelOf(failedLine))
	require.NotContains(t, fmt.Sprint(failedLine), "test-secret")
	auth, ok := rawVal(failedLine, "Authorization")
	require.True(t, ok)
	require.Equal(t, Redacted, auth)
	title, ok := rawVal(failedLine, "dashboardTitle")
	require.True(t, ok)
	require.Equal(t, "Sales %s", title)
	nested, ok := rawVal(failedLine, "nested")
	require.True(t, ok)
	nestedMap, ok := nested.(map[string]any)
	require.True(t, ok)
	require.Equal(t, Redacted, nestedMap["token"])
	require.Equal(t, "ok", nestedMap["name"])
	inner, ok := nestedMap["inner"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, Redacted, inner["password"])

	childLine := lineWithMsg(t, scenario.loggedArgs, "child")
	require.Equal(t, "info", levelOf(childLine))
	id, ok := rawVal(childLine, "req.http.id")
	require.True(t, ok)
	require.Equal(t, "abc", id)
	method, ok := rawVal(childLine, "req.http.method")
	require.True(t, ok)
	require.Equal(t, "GET", method)
}

func lineWithMsgPrefix(t *testing.T, lines [][]any, prefix string) []any {
	t.Helper()
	for _, line := range lines {
		got, ok := rawVal(line, "msg")
		if ok && len(fmt.Sprint(got)) >= len(prefix) && fmt.Sprint(got)[:len(prefix)] == prefix {
			return line
		}
	}
	t.Fatalf("no log line with msg prefix %q", prefix)
	return nil
}

func lineWithMsg(t *testing.T, lines [][]any, msg string) []any {
	t.Helper()
	for _, line := range lines {
		got, ok := rawVal(line, "msg")
		if ok && fmt.Sprint(got) == msg {
			return line
		}
	}
	t.Fatalf("no log line with msg %q in %#v", msg, lines)
	return nil
}

func levelOf(line []any) string {
	v, ok := rawVal(line, "lvl")
	if !ok {
		return ""
	}
	return fmt.Sprint(v)
}

func rawVal(line []any, key string) (any, bool) {
	for i := 0; i+1 < len(line); i += 2 {
		if fmt.Sprint(line[i]) == key {
			return line[i+1], true
		}
	}
	return nil, false
}
