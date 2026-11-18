package redact

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"testing/slogtest"
)

type secretVal string

func (secretVal) IsLogSecret() {}

func (s secretVal) LogValue() slog.Value {
	// If this is called, the secret leaked into the log pipeline.
	return slog.StringValue("LEAKED:" + string(s))
}

func TestParseKeys(t *testing.T) {
	t.Parallel()
	if ParseKeys("") != nil {
		t.Fatalf("empty input: got %#v", ParseKeys(""))
	}
	got := ParseKeys("ssn, session_id   credit-card")
	want := []string{"ssn", "session_id", "credit-card"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseKeys = %#v, want %#v", got, want)
	}
}

func TestDefaultKeys(t *testing.T) {
	t.Parallel()
	keys := DefaultKeys()
	if !reflect.DeepEqual(keys, defaultKeys) {
		t.Fatalf("DefaultKeys = %#v", keys)
	}
	keys[0] = "mutated"
	if DefaultKeys()[0] != "password" {
		t.Fatal("DefaultKeys must return a copy")
	}
}

func TestRedactsSensitiveKeys(t *testing.T) {
	t.Parallel()

	keys := []string{
		"password",
		"Password",
		"token",
		"access_token",
		"Authorization",
		"cookie",
		"api_key",
		"X-API-Key",
		"private_key",
		"secret",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			got := capture(t, nil, func(l *slog.Logger) {
				l.Info("x", key, "super-secret-value")
			})
			if got[key] != Replacement {
				t.Fatalf("key %q: got %#v", key, got[key])
			}
			if contains(mustMarshal(got), "super-secret-value") {
				t.Fatal("plaintext secret leaked")
			}
		})
	}
}

func TestDoesNotRedactUnrelatedKeys(t *testing.T) {
	t.Parallel()
	got := capture(t, nil, func(l *slog.Logger) {
		l.Info("x", "user", "nick", "org", 1, "key", "visible", "notoken", "ok")
	})
	if got["user"] != "nick" || got["key"] != "visible" || got["notoken"] != "ok" {
		t.Fatalf("unexpected attrs: %#v", got)
	}
	org, ok := got["org"].(float64)
	if !ok || org != 1 {
		t.Fatalf("org = %#v", got["org"])
	}
}

func TestExtraKeysFromConfig(t *testing.T) {
	t.Parallel()
	got := capture(t, ParseKeys("ssn, session_id"), func(l *slog.Logger) {
		l.Info("x", "ssn", "111-11-1111", "session_id", "abc", "user", "nick")
	})
	if got["ssn"] != Replacement || got["session_id"] != Replacement {
		t.Fatalf("extra keys not redacted: %#v", got)
	}
	if got["user"] != "nick" {
		t.Fatalf("user = %#v", got["user"])
	}
}

func TestNestedGroups(t *testing.T) {
	t.Parallel()
	got := capture(t, nil, func(l *slog.Logger) {
		l.Info("x", slog.Group("db", slog.String("password", "hunter2"), slog.Int("port", 5432)))
	})
	db, ok := got["db"].(map[string]any)
	if !ok {
		t.Fatalf("db group missing: %#v", got)
	}
	if db["password"] != Replacement {
		t.Fatalf("password = %#v", db["password"])
	}
	port, ok := db["port"].(float64)
	if !ok || port != 5432 {
		t.Fatalf("port = %#v", db["port"])
	}
}

func TestWithGroup(t *testing.T) {
	t.Parallel()
	got := capture(t, nil, func(l *slog.Logger) {
		l.WithGroup("auth").Info("x", "token", "abc", "user", "nick")
	})
	auth, ok := got["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth group missing: %#v", got)
	}
	if auth["token"] != Replacement || auth["user"] != "nick" {
		t.Fatalf("auth = %#v", auth)
	}
}

func TestWithAttrs(t *testing.T) {
	t.Parallel()
	got := capture(t, nil, func(l *slog.Logger) {
		l.With("password", "hunter2", "user", "nick").Info("x")
	})
	if got["password"] != Replacement || got["user"] != "nick" {
		t.Fatalf("got %#v", got)
	}
}

func TestSecretLogValuerIsNotResolved(t *testing.T) {
	t.Parallel()
	got := capture(t, nil, func(l *slog.Logger) {
		l.Info("x", "credential", secretVal("hunter2"))
	})
	if got["credential"] != Replacement {
		t.Fatalf("credential = %#v", got["credential"])
	}
	raw := mustMarshal(got)
	if contains(raw, "hunter2") || contains(raw, "LEAKED") {
		t.Fatalf("secret leaked: %s", raw)
	}
}

func TestSensitiveKeyLogValuerIsNotResolved(t *testing.T) {
	t.Parallel()
	got := capture(t, nil, func(l *slog.Logger) {
		l.Info("x", "password", secretVal("hunter2"))
	})
	if got["password"] != Replacement {
		t.Fatalf("password = %#v", got["password"])
	}
	if contains(mustMarshal(got), "LEAKED") {
		t.Fatal("LogValue was called")
	}
}

func TestSlogHandlerContract(t *testing.T) {
	var buf bytes.Buffer
	h := New(slog.NewJSONHandler(&buf, nil))
	results := func() []map[string]any {
		var out []map[string]any
		for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatal(err)
			}
			out = append(out, m)
		}
		return out
	}
	if err := slogtest.TestHandler(h, results); err != nil {
		t.Fatal(err)
	}
}

func TestNewPanicsOnNilNext(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	New(nil)
}

func TestHandlerEnabledDelegates(t *testing.T) {
	t.Parallel()
	inner := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := New(inner)
	if h.Enabled(t.Context(), slog.LevelInfo) {
		t.Fatal("info should be disabled")
	}
	if !h.Enabled(t.Context(), slog.LevelError) {
		t.Fatal("error should be enabled")
	}
}

func TestEmptyWithGroupAndAttrs(t *testing.T) {
	t.Parallel()
	h := New(slog.NewJSONHandler(io.Discard, nil))
	if h.WithGroup("") != h {
		t.Fatal("empty group should return the same handler")
	}
	if h.WithAttrs(nil) != h {
		t.Fatal("empty attrs should return the same handler")
	}
}

func capture(t *testing.T, extra []string, fn func(*slog.Logger)) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	h := New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: dropTime}), extra...)
	fn(slog.New(h))
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func dropTime(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}

func mustMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func contains(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}
