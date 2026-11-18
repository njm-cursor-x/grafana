package logctx

import (
	"context"
	"log/slog"
	"testing"

	"github.com/grafana/grafana/pkg/infra/log/logkeys"
)

func TestIdentitySetters(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if _, ok := Org(ctx); ok {
		t.Fatal("expected no org")
	}
	if _, ok := User(ctx); ok {
		t.Fatal("expected no user")
	}
	if _, ok := RequestID(ctx); ok {
		t.Fatal("expected no request id")
	}

	ctx = WithOrg(ctx, 0)
	orgID, ok := Org(ctx)
	if !ok || orgID != 0 {
		t.Fatalf("org=%d ok=%v", orgID, ok)
	}

	ctx = WithOrg(ctx, 42)
	ctx = WithUser(ctx, 7)
	ctx = WithRequestID(ctx, "req-1")

	orgID, ok = Org(ctx)
	if !ok || orgID != 42 {
		t.Fatalf("org=%d ok=%v", orgID, ok)
	}
	userID, ok := User(ctx)
	if !ok || userID != 7 {
		t.Fatalf("user=%d ok=%v", userID, ok)
	}
	rid, ok := RequestID(ctx)
	if !ok || rid != "req-1" {
		t.Fatalf("request id=%q ok=%v", rid, ok)
	}
}

func TestWithAttrsAppends(t *testing.T) {
	t.Parallel()

	ctx := WithAttrs(context.Background(), slog.String("a", "1"))
	ctx = WithAttrs(ctx, slog.Int("b", 2))

	attrs := Attrs(ctx)
	if len(attrs) != 2 || attrs[0].Key != "a" || attrs[0].Value.String() != "1" || attrs[1].Key != "b" {
		t.Fatalf("attrs=%v", attrs)
	}

	attrs[0] = slog.String("a", "mutated")
	if Attrs(ctx)[0].Value.String() != "1" {
		t.Fatal("Attrs should return a copy")
	}
}

func TestAttrsIncludesIdentity(t *testing.T) {
	t.Parallel()

	ctx := WithOrg(context.Background(), 3)
	ctx = WithUser(ctx, 9)
	ctx = WithRequestID(ctx, "abc")
	ctx = WithAttrs(ctx, slog.String("rule", "r1"))

	attrs := Attrs(ctx)
	want := []slog.Attr{
		slog.Int64(logkeys.KeyOrgID, 3),
		slog.Int64(logkeys.KeyUserID, 9),
		slog.String(logkeys.KeyRequestID, "abc"),
		slog.String("rule", "r1"),
	}
	if len(attrs) != len(want) {
		t.Fatalf("len=%d want %d (%v)", len(attrs), len(want), attrs)
	}
	for i := range want {
		if attrs[i].Key != want[i].Key || attrs[i].Value.String() != want[i].Value.String() {
			t.Fatalf("attrs[%d]=%v want %v", i, attrs[i], want[i])
		}
	}
}

func TestExtractMatchesAttrs(t *testing.T) {
	t.Parallel()
	ctx := WithOrg(context.Background(), 1)
	attrs := Attrs(ctx)
	got := Extract(ctx)
	if len(got) != len(attrs) {
		t.Fatalf("Extract len=%d Attrs len=%d", len(got), len(attrs))
	}
	for i := range attrs {
		if got[i].Key != attrs[i].Key {
			t.Fatalf("Extract[%d]=%v want %v", i, got[i], attrs[i])
		}
	}
}

func TestEmptyRequestIDIsAbsent(t *testing.T) {
	t.Parallel()

	ctx := WithRequestID(context.Background(), "")
	if _, ok := RequestID(ctx); ok {
		t.Fatal("empty request id should be absent")
	}
	if len(Attrs(ctx)) != 0 {
		t.Fatalf("attrs=%v", Attrs(ctx))
	}
}
