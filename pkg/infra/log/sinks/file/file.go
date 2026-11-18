// Package file implements the file log sink, including the in-house rotator that
// preserves the existing [log.file] keys (log_rotate, max_lines, max_size_shift,
// daily_rotate, max_days, file_name). Writes are synchronous.
package file

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/grafana/grafana/pkg/infra/log/slogfmt"
)

// Config is the [log.file] section. MaxSize is the byte limit after applying max_size_shift.
type Config struct {
	Format   string
	Filename string
	Rotate   bool
	MaxLines int
	MaxSize  int
	Daily    bool
	MaxDays  int64
	// Now overrides time.Now, for rotation tests.
	Now func() time.Time
}

// New returns a handler that writes formatted records to Filename, rotating when the
// configured limits are hit. The caller must Close it.
func New(cfg Config) (slog.Handler, error) {
	if cfg.Filename == "" {
		return nil, errors.New("config must have filename")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Filename), 0o750); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	w := &writer{cfg: cfg}
	if err := w.open(); err != nil {
		return nil, err
	}
	return &Handler{inner: slogfmt.New(cfg.Format, w), w: w}, nil
}

// Handler writes slog records to a rotating log file.
type Handler struct {
	inner slog.Handler
	w     *writer
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return &Handler{inner: h.inner.WithAttrs(attrs), w: h.w}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &Handler{inner: h.inner.WithGroup(name), w: h.w}
}

func (h *Handler) Flush(context.Context) error {
	return h.w.sync()
}

func (h *Handler) Reload() error {
	return h.w.reload()
}

func (h *Handler) Close(context.Context) error {
	return h.w.close()
}

type writer struct {
	cfg Config

	startMu sync.Mutex
	mu      sync.Mutex
	fd      *os.File

	lines    int
	size     int
	openDate int
}

func (w *writer) now() time.Time {
	return w.cfg.Now()
}

func (w *writer) Write(b []byte) (int, error) {
	w.check(len(b))
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fd == nil {
		return 0, errors.New("log file is closed")
	}
	return w.fd.Write(b)
}

func (w *writer) check(size int) {
	w.startMu.Lock()
	defer w.startMu.Unlock()

	if w.cfg.Rotate && ((w.cfg.MaxLines > 0 && w.lines >= w.cfg.MaxLines) ||
		(w.cfg.MaxSize > 0 && w.size >= w.cfg.MaxSize) ||
		(w.cfg.Daily && w.now().Day() != w.openDate)) {
		if err := w.rotate(); err != nil {
			fmt.Fprintf(os.Stderr, "file log (%q): %s\n", w.cfg.Filename, err)
			return
		}
	}
	w.lines++
	w.size += size
}

func (w *writer) create() (*os.File, error) {
	//nolint:gosec // log path comes from operator-controlled Grafana config
	return os.OpenFile(w.cfg.Filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
}

func (w *writer) setFD(fd *os.File) error {
	if w.fd != nil {
		if err := w.fd.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			return fmt.Errorf("closing old log file: %w", err)
		}
	}
	w.fd = fd
	return nil
}

func (w *writer) open() error {
	fd, err := w.create()
	if err != nil {
		return err
	}
	if err := w.setFD(fd); err != nil {
		_ = fd.Close()
		return err
	}
	return w.initFD()
}

func (w *writer) lineCount() (int, error) {
	r, err := os.Open(w.cfg.Filename)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %q: %w", w.cfg.Filename, err)
	}
	defer r.Close()

	buf := make([]byte, 32*1024)
	count := 0
	for {
		c, err := r.Read(buf)
		count += bytes.Count(buf[:c], []byte{'\n'})
		if err != nil {
			if errors.Is(err, io.EOF) {
				return count, nil
			}
			return 0, err
		}
	}
}

func (w *writer) initFD() error {
	info, err := w.fd.Stat()
	if err != nil {
		return fmt.Errorf("get stat: %s", err)
	}
	w.size = int(info.Size())
	w.openDate = w.now().Day()
	if info.Size() > 0 {
		count, err := w.lineCount()
		if err != nil {
			return err
		}
		w.lines = count
	} else {
		w.lines = 0
	}
	return nil
}

func (w *writer) rotate() error {
	_, err := os.Lstat(w.cfg.Filename)
	if err != nil {
		return nil
	}

	num := 1
	fname := ""
	for ; err == nil && num <= 999; num++ {
		fname = w.cfg.Filename + fmt.Sprintf(".%s.%03d", w.now().Format("2006-01-02"), num)
		_, err = os.Lstat(fname)
	}
	if err == nil {
		return fmt.Errorf("rotate: cannot find free log number to rename %s", w.cfg.Filename)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.fd != nil {
		if err := w.fd.Close(); err != nil {
			return err
		}
		w.fd = nil
	}

	if err = os.Rename(w.cfg.Filename, fname); err != nil {
		return fmt.Errorf("rotate: %s", err)
	}
	if err = w.open(); err != nil {
		return fmt.Errorf("rotate open: %s", err)
	}
	go w.deleteOldLogs()
	return nil
}

func (w *writer) deleteOldLogs() {
	dir := filepath.Dir(w.cfg.Filename)
	cutoff := w.now().Unix() - 60*60*24*w.cfg.MaxDays
	prefix := filepath.Base(w.cfg.Filename)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) (returnErr error) {
		defer func() {
			if r := recover(); r != nil {
				returnErr = fmt.Errorf("unable to delete old log '%s', error: %+v", path, r)
			}
		}()
		if err != nil || info == nil {
			return err
		}
		if !info.IsDir() && info.ModTime().Unix() < cutoff &&
			strings.HasPrefix(filepath.Base(path), prefix) {
			return os.Remove(path) // #nosec G122 -- log rotation on operator-controlled directory
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "file log (%q): %s\n", w.cfg.Filename, err)
	}
}

func (w *writer) sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fd == nil {
		return nil
	}
	return w.fd.Sync()
}

func (w *writer) reload() error {
	w.startMu.Lock()
	defer w.startMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.fd != nil {
		if err := w.fd.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			return err
		}
		w.fd = nil
	}
	return w.open()
}

func (w *writer) close() error {
	w.startMu.Lock()
	defer w.startMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fd == nil {
		return nil
	}
	err := w.fd.Close()
	w.fd = nil
	return err
}
