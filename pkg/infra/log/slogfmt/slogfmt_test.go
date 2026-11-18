package slogfmt

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"testing/slogtest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nilErr struct{}

func (*nilErr) Error() string { panic("nil receiver") }

func TestText(t *testing.T) {
	buf := &bytes.Buffer{}
	ts := time.Date(2026, 11, 18, 13, 0, 0, 123456789, time.UTC)
	r := slog.NewRecord(ts, slog.LevelWarn, "hello world", 0)
	r.AddAttrs(
		slog.String("logger", "sqlstore"),
		slog.Any("error", errors.New("bad thing")),
		slog.Group("g", slog.Int("n", 1)),
	)
	require.NoError(t, NewText(buf).Handle(t.Context(), r))

	assert.Equal(t,
		`t=2026-11-18T13:00:00.123456789Z level=warn msg="hello world" logger=sqlstore error="bad thing" g.n=1`+"\n",
		buf.String())
}

func TestJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	var typedNil *nilErr
	ts := time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC)
	r := slog.NewRecord(ts, slog.LevelDebug, "hi", 0)
	r.AddAttrs(slog.Any("error", errors.New("bad")), slog.Any("other", error(typedNil)))
	require.NoError(t, NewJSON(buf).WithAttrs([]slog.Attr{slog.String("logger", "api")}).Handle(t.Context(), r))

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, map[string]any{
		"t":      "2026-11-18T13:00:00Z",
		"level":  "debug",
		"msg":    "hi",
		"logger": "api",
		"error":  "bad",
		"other":  "<error: nil receiver>",
	}, got)
}

func TestNewSelectsFormat(t *testing.T) {
	for format, wantJSON := range map[string]bool{"json": true, " JSON ": true, "text": false, "console": false, "": false} {
		buf := &bytes.Buffer{}
		require.NoError(t, New(format, buf).Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "m", 0)))
		assert.Equal(t, wantJSON, strings.HasPrefix(buf.String(), "{"), "format %q", format)
	}
}

func TestAcceptsAllLevels(t *testing.T) {
	assert.True(t, NewText(&bytes.Buffer{}).Enabled(t.Context(), slog.LevelDebug-100))
}

func TestJSONHandlerConformance(t *testing.T) {
	buf := &bytes.Buffer{}
	results := func() []map[string]any {
		lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
		out := make([]map[string]any, 0, len(lines))
		for _, line := range lines {
			m := map[string]any{}
			require.NoError(t, json.Unmarshal(line, &m))
			if v, ok := m[TimeKey]; ok {
				m[slog.TimeKey] = v
				delete(m, TimeKey)
			}
			out = append(out, m)
		}
		return out
	}
	require.NoError(t, slogtest.TestHandler(NewJSON(buf), results))
}
