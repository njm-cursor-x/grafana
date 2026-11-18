package log

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// go test ./pkg/infra/log/ -run TestGoldenOutput -update-golden
var updateGolden = flag.Bool("update-golden", false, "regenerate pkg/infra/log testdata golden snapshots")

const updateGoldenCommand = "go test ./pkg/infra/log/ -run TestGoldenOutput -update-golden"

type goldenCase struct {
	name    string
	level   string
	filters map[string]string
	emit    func()
}

type sampleStruct struct {
	Name  string
	Count int
}

// goldenCases is the compatibility contract. Later engine PRs must keep these
// records stable (legacy engine / legacy field names). Do not rewrite cases
// when swapping the installer.
func goldenCases() []goldenCase {
	return []goldenCase{
		{
			name:  "levels",
			level: "debug",
			emit: func() {
				l := New("levels")
				l.Debug("debug message", "k", "v")
				l.Info("info message", "k", "v")
				l.Warn("warn message", "k", "v")
				l.Error("error message", "k", "v")
			},
		},
		{
			name: "named-loggers",
			emit: func() {
				New("http.server").Info("listening", "addr", ":3000")
				New("sqlstore").Info("connected", "db", "sqlite")
			},
		},
		{
			name:    "filters",
			level:   "info",
			filters: map[string]string{"sqlstore": "debug"},
			emit: func() {
				New("sqlstore").Debug("query", "sql", "SELECT 1")
				New("http.server").Debug("dropped")
				New("http.server").Info("served", "status", 200)
			},
		},
		{
			name: "errors",
			emit: func() {
				New("errors").Error("failed", "error", errors.New("boom"), "attempt", 3)
			},
		},
		{
			name: "structs",
			emit: func() {
				New("structs").Info("payload",
					"data", sampleStruct{Name: "foo", Count: 3},
					"tags", []string{"a", "b"},
				)
			},
		},
		{
			name: "nil-values",
			emit: func() {
				var err error
				var ptr *string
				New("nils").Info("nils", "err", err, "ptr", ptr, "val", nil)
			},
		},
		{
			name: "contextual-attributes",
			emit: func() {
				ctx := WithContextualAttributes(context.Background(), []any{
					"traceID", "trace-abc",
					"orgId", int64(1),
					"userId", int64(42),
				})
				New("request").FromContext(ctx).Info("handled", "path", "/api/dashboards")
			},
		},
		{
			name: "typed-values",
			emit: func() {
				New("types").Info("mixed",
					"ok", true,
					"n", 7,
					"ratio", 1.5,
					"note", "has spaces",
				)
			},
		},
		{
			name: "child-logger",
			emit: func() {
				New("parent").New("requestID", "r1").Info("child", "k", "v")
			},
		},
	}
}

func TestGoldenOutput(t *testing.T) {
	// JSON snapshots use go-kit's JSON logger, which encodes via a map so keys
	// are sorted by encoding/json. slog.JSONHandler preserves attribute order;
	// a slog replacement must keep this sorted-key layout to match the files.
	for _, format := range goldenFormats {
		t.Run(format, func(t *testing.T) {
			got := renderGolden(t, format)
			path := filepath.Join("testdata", "golden", format+".golden")
			compareOrUpdateGolden(t, path, got)
		})
	}
}

func renderGolden(t *testing.T, format string) string {
	t.Helper()
	var b strings.Builder
	for i, tc := range goldenCases() {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("=== ")
		b.WriteString(tc.name)
		b.WriteString(" ===\n")
		b.WriteString(runGoldenCase(t, format, tc))
	}
	return b.String()
}

func runGoldenCase(t *testing.T, format string, tc goldenCase) string {
	t.Helper()
	var buf bytes.Buffer
	installCurrentEngine(t, &buf, engineOpts{
		format:  format,
		level:   tc.level,
		filters: tc.filters,
	})
	tc.emit()
	return buf.String()
}

func compareOrUpdateGolden(t *testing.T, path string, got string) {
	t.Helper()
	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // path is a hardcoded testdata file
	require.NoError(t, err, "missing golden file %s; regenerate with:\n\n\t%s", path, updateGoldenCommand)
	assert.Equal(t, string(want), got, "golden snapshot changed; if intended, rerun with -update-golden")
}
