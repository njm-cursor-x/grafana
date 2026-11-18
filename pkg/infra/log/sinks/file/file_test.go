package file

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/slogtest"
	"time"

	"github.com/grafana/grafana/pkg/infra/log/slogfmt"
)

type lifecycle interface {
	Flush(context.Context) error
	Reload() error
	Close(context.Context) error
}

var _ lifecycle = (*Handler)(nil)

func TestWritesJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grafana.log")
	h, err := New(Config{Filename: path, Format: "json", Rotate: false})
	if err != nil {
		t.Fatal(err)
	}
	fh := h.(*Handler)
	t.Cleanup(func() {
		if err := fh.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})

	r := slog.NewRecord(time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC), slog.LevelError, "written", 0)
	r.AddAttrs(slog.String("logger", "slogtest.file"), slog.Int("grafana.org.id", 4))
	if err := h.Handle(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if err := fh.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}

	var line map[string]any
	if err := json.Unmarshal(readFile(t, path), &line); err != nil {
		t.Fatal(err)
	}
	if line["level"] != "error" || line["msg"] != "written" || line["logger"] != "slogtest.file" || line["grafana.org.id"] != 4.0 {
		t.Fatalf("unexpected record: %#v", line)
	}
}

func TestRotatesByLineCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grafana.log")
	h, err := New(Config{Filename: path, Format: "text", Rotate: true, MaxLines: 2, MaxSize: 1 << 28})
	if err != nil {
		t.Fatal(err)
	}
	fh := h.(*Handler)
	t.Cleanup(func() { _ = fh.Close(t.Context()) })

	writeN(t, h, 3)
	if err := fh.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := lineCount(t, path); got != 1 {
		t.Fatalf("current file lines = %d, want 1", got)
	}
	rotated := findRotated(t, dir, "grafana.log")
	if rotated == "" {
		t.Fatal("expected a rotated file")
	}
	if got := lineCount(t, rotated); got != 2 {
		t.Fatalf("rotated file lines = %d, want 2", got)
	}
}

func TestRotatesBySize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grafana.log")
	h, err := New(Config{Filename: path, Format: "json", Rotate: true, MaxLines: 1_000_000, MaxSize: 80})
	if err != nil {
		t.Fatal(err)
	}
	fh := h.(*Handler)
	t.Cleanup(func() { _ = fh.Close(t.Context()) })

	writeN(t, h, 8)
	if err := fh.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if findRotated(t, dir, "grafana.log") == "" {
		t.Fatal("expected a rotated file")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if int(info.Size()) >= 80*4 {
		t.Fatalf("current file still large: %d", info.Size())
	}
}

func TestRotatesDaily(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grafana.log")
	day1 := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 11, 19, 1, 0, 0, 0, time.UTC)
	now := day1
	h, err := New(Config{
		Filename: path,
		Format:   "text",
		Rotate:   true,
		MaxLines: 1_000_000,
		MaxSize:  1 << 28,
		Daily:    true,
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	fh := h.(*Handler)
	t.Cleanup(func() { _ = fh.Close(t.Context()) })

	writeN(t, h, 1)
	now = day2
	writeN(t, h, 1)

	rotated := findRotated(t, dir, "grafana.log")
	if !strings.Contains(filepath.Base(rotated), "2026-11-19") {
		t.Fatalf("rotated name %q does not include 2026-11-19", rotated)
	}
	if lineCount(t, path) != 1 || lineCount(t, rotated) != 1 {
		t.Fatalf("line counts current=%d rotated=%d", lineCount(t, path), lineCount(t, rotated))
	}
}

func TestDeletesOldLogs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grafana.log")
	old := filepath.Join(dir, "grafana.log.2020-01-01.001")
	if err := os.WriteFile(old, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	h, err := New(Config{
		Filename: path,
		Format:   "text",
		Rotate:   true,
		MaxLines: 1,
		MaxSize:  1 << 28,
		MaxDays:  7,
	})
	if err != nil {
		t.Fatal(err)
	}
	fh := h.(*Handler)
	t.Cleanup(func() { _ = fh.Close(t.Context()) })

	writeN(t, h, 2)

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := os.Stat(old)
		if os.IsNotExist(err) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("old log was not deleted")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReloadReopensFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grafana.log")
	h, err := New(Config{Filename: path, Format: "text", Rotate: false})
	if err != nil {
		t.Fatal(err)
	}
	fh := h.(*Handler)
	t.Cleanup(func() { _ = fh.Close(t.Context()) })

	writeN(t, h, 1)
	moved := path + ".moved"
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	if err := fh.Reload(); err != nil {
		t.Fatal(err)
	}
	writeN(t, h, 1)

	if lineCount(t, moved) != 1 || lineCount(t, path) != 1 {
		t.Fatalf("after reload moved=%d current=%d", lineCount(t, moved), lineCount(t, path))
	}
}

func TestCloseStopsWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grafana.log")
	h, err := New(Config{Filename: path, Format: "text", Rotate: false})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.(*Handler).Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "after close", 0)); err == nil {
		t.Fatal("expected write after close to fail")
	}
}

func TestConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grafana.log")
	h, err := New(Config{Filename: path, Format: "text", Rotate: false})
	if err != nil {
		t.Fatal(err)
	}
	fh := h.(*Handler)
	t.Cleanup(func() { _ = fh.Close(t.Context()) })

	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			errCh <- h.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "n", 0))
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := lineCount(t, path); got != 20 {
		t.Fatalf("lines = %d, want 20", got)
	}
}

func TestJSONHandlerConformance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grafana.log")
	h, err := New(Config{Filename: path, Format: "json", Rotate: false})
	if err != nil {
		t.Fatal(err)
	}
	fh := h.(*Handler)
	t.Cleanup(func() { _ = fh.Close(t.Context()) })

	results := func() []map[string]any {
		raw := bytes.Split(bytes.TrimSpace(readFile(t, path)), []byte("\n"))
		out := make([]map[string]any, 0, len(raw))
		for _, line := range raw {
			if len(line) == 0 {
				continue
			}
			m := map[string]any{}
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatal(err)
			}
			if v, ok := m[slogfmt.TimeKey]; ok {
				m[slog.TimeKey] = v
				delete(m, slogfmt.TimeKey)
			}
			out = append(out, m)
		}
		return out
	}
	if err := slogtest.TestHandler(h, results); err != nil {
		t.Fatal(err)
	}
}

func writeN(t *testing.T, h slog.Handler, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := h.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "line", 0)); err != nil {
			t.Fatal(err)
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func lineCount(t *testing.T, path string) int {
	t.Helper()
	b := bytes.TrimSpace(readFile(t, path))
	if len(b) == 0 {
		return 0
	}
	return bytes.Count(b, []byte("\n")) + 1
}

func findRotated(t *testing.T, dir, base string) string {
	t.Helper()
	var found string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if name != base && strings.HasPrefix(name, base+".") {
			found = path
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}
