//go:build !windows && !nacl && !plan9

package syslog

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

type lifecycle interface {
	Reload() error
	Close(context.Context) error
}

var _ lifecycle = (*Handler)(nil)

func TestWritesPriorityFromLevel(t *testing.T) {
	ln := listenUDP(t)
	h, err := New(Config{
		Format:   "json",
		Network:  "udp",
		Address:  ln.LocalAddr().String(),
		Facility: "local7",
		Tag:      "gf",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.(*Handler).Close(t.Context()) })

	r := slog.NewRecord(time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC), slog.LevelError, "boom", 0)
	r.AddAttrs(slog.String("logger", "syslogtest"))
	if err := h.Handle(t.Context(), r); err != nil {
		t.Fatal(err)
	}

	pkt := readPkt(t, ln)
	for _, want := range []string{"<187>", "gf[", `"msg":"boom"`, `"logger":"syslogtest"`} {
		if !strings.Contains(pkt, want) {
			t.Fatalf("packet %q missing %q", pkt, want)
		}
	}
	idx := strings.Index(pkt, "{")
	if idx <= 0 {
		t.Fatalf("no JSON payload in %q", pkt)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(pkt[idx:]), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["level"] != "error" {
		t.Fatalf("level = %v", payload["level"])
	}
}

func TestMapsLevels(t *testing.T) {
	ln := listenUDP(t)
	h, err := New(Config{Format: "text", Network: "udp", Address: ln.LocalAddr().String(), Tag: "gf"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.(*Handler).Close(t.Context()) })

	cases := []struct {
		level slog.Level
		pri   string
	}{
		{slog.LevelDebug, "<191>"},
		{slog.LevelInfo, "<190>"},
		{slog.LevelWarn, "<188>"},
		{slog.LevelError, "<187>"},
	}
	for _, tc := range cases {
		if err := h.Handle(t.Context(), slog.NewRecord(time.Now(), tc.level, tc.level.String(), 0)); err != nil {
			t.Fatal(err)
		}
		pkt := readPkt(t, ln)
		if !strings.HasPrefix(pkt, tc.pri) {
			t.Fatalf("level %s: packet %q, want prefix %s", tc.level, pkt, tc.pri)
		}
	}
}

func TestReloadRedials(t *testing.T) {
	ln1 := listenUDP(t)
	h, err := New(Config{Format: "text", Network: "udp", Address: ln1.LocalAddr().String(), Tag: "gf"})
	if err != nil {
		t.Fatal(err)
	}
	fh := h.(*Handler)
	t.Cleanup(func() { _ = fh.Close(t.Context()) })

	if err := h.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "first", 0)); err != nil {
		t.Fatal(err)
	}
	if pkt := readPkt(t, ln1); !strings.Contains(pkt, "first") {
		t.Fatalf("missing first: %q", pkt)
	}

	ln2 := listenUDP(t)
	fh.conn.cfg.Address = ln2.LocalAddr().String()
	if err := fh.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "second", 0)); err != nil {
		t.Fatal(err)
	}
	if pkt := readPkt(t, ln2); !strings.Contains(pkt, "second") {
		t.Fatalf("missing second: %q", pkt)
	}
}

func listenUDP(t *testing.T) *net.UDPConn {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	if err := ln.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return ln
}

func readPkt(t *testing.T, ln *net.UDPConn) string {
	t.Helper()
	if err := ln.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	n, _, err := ln.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	return string(buf[:n])
}
