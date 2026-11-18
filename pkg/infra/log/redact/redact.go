// Package redact provides an slog.Handler that masks sensitive attribute values
// before they reach a sink. It is a safety net, not a guarantee: call sites
// should still avoid logging secrets.
//
// Construct with New and compose it in front of other handlers. The slog
// pipeline installs this as middleware on SlogManager.
package redact

import (
	"context"
	"log/slog"
	"strings"
	"unicode"
)

// Replacement is the placeholder written in place of a redacted value.
const Replacement = "[REDACTED]"

// defaultKeys is the built-in deny-list. Matching is case-insensitive and
// ignores '-', '_', and other separators (so "api_key" and "X-API-Key" both
// match "apikey").
var defaultKeys = []string{
	"password",
	"secret",
	"token",
	"authorization",
	"cookie",
	"apikey",
	"private_key",
}

// Secret is a marker for values that wrap credentials. The handler redacts
// them regardless of attribute key and does not call slog.LogValuer, so a
// LogValue implementation cannot leak the secret.
type Secret interface {
	IsLogSecret()
}

// DefaultKeys returns a copy of the built-in deny-list.
func DefaultKeys() []string {
	out := make([]string, len(defaultKeys))
	copy(out, defaultKeys)
	return out
}

// ParseKeys splits a [log] redact_keys value on commas and whitespace.
func ParseKeys(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Handler wraps a slog.Handler and masks values of sensitive keys
// (including keys nested in slog groups) before the record is forwarded.
type Handler struct {
	next slog.Handler
	keys map[string]struct{}
}

var _ slog.Handler = (*Handler)(nil)

// New returns a handler that redacts defaultKeys plus extraKeys.
// extraKeys typically come from [log] redact_keys.
func New(next slog.Handler, extraKeys ...string) *Handler {
	if next == nil {
		panic("redact: next handler is nil")
	}
	keys := make(map[string]struct{}, len(defaultKeys)+len(extraKeys))
	for _, k := range defaultKeys {
		addKey(keys, k)
	}
	for _, k := range extraKeys {
		addKey(keys, k)
	}
	return &Handler{next: next, keys: keys}
}

func addKey(keys map[string]struct{}, k string) {
	k = strings.TrimSpace(k)
	if k == "" {
		return
	}
	// Only the full normalized key is stored. Matching still looks at
	// attribute-key segments (access_token → token) against this set.
	keys[normalize(k)] = struct{}{}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if r.NumAttrs() == 0 {
		return h.next.Handle(ctx, r)
	}
	needs := false
	r.Attrs(func(a slog.Attr) bool {
		if h.attrNeedsRedact(a) {
			needs = true
			return false
		}
		return true
	})
	if !needs {
		return h.next.Handle(ctx, r)
	}

	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(h.redactAttr(a))
		return true
	})
	return h.next.Handle(ctx, nr)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = h.redactAttr(a)
	}
	return h.clone(h.next.WithAttrs(out))
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.clone(h.next.WithGroup(name))
}

func (h *Handler) clone(next slog.Handler) *Handler {
	return &Handler{next: next, keys: h.keys}
}

func (h *Handler) attrNeedsRedact(a slog.Attr) bool {
	v := a.Value
	switch v.Kind() {
	case slog.KindLogValuer:
		if h.matchKey(a.Key) || isSecret(v.Any()) {
			return true
		}
		v = v.Resolve()
		if isSecret(v.Any()) {
			return true
		}
		if v.Kind() == slog.KindGroup {
			return h.groupNeedsRedact(a.Key, v)
		}
		return h.matchKey(a.Key)
	case slog.KindAny:
		if isSecret(v.Any()) {
			return true
		}
		return h.matchKey(a.Key)
	case slog.KindGroup:
		return h.groupNeedsRedact(a.Key, v)
	default:
		return h.matchKey(a.Key)
	}
}

func (h *Handler) groupNeedsRedact(key string, v slog.Value) bool {
	if key != "" && h.matchKey(key) {
		return true
	}
	for _, ga := range v.Group() {
		if h.attrNeedsRedact(ga) {
			return true
		}
	}
	return false
}

func (h *Handler) redactAttr(a slog.Attr) slog.Attr {
	v := a.Value
	if v.Kind() == slog.KindLogValuer {
		// Do not call LogValue for secrets or sensitive keys: the valuer
		// might return the plaintext credential.
		if h.matchKey(a.Key) || isSecret(v.Any()) {
			return slog.String(a.Key, Replacement)
		}
		v = v.Resolve()
		a.Value = v
	}
	if (v.Kind() == slog.KindAny || v.Kind() == slog.KindLogValuer) && isSecret(v.Any()) {
		return slog.String(a.Key, Replacement)
	}
	if v.Kind() == slog.KindGroup {
		if a.Key != "" && h.matchKey(a.Key) {
			return slog.String(a.Key, Replacement)
		}
		g := v.Group()
		out := make([]slog.Attr, len(g))
		for i, ga := range g {
			out[i] = h.redactAttr(ga)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	}
	if h.matchKey(a.Key) {
		return slog.String(a.Key, Replacement)
	}
	return a
}

func (h *Handler) matchKey(key string) bool {
	if key == "" || len(h.keys) == 0 {
		return false
	}
	if isASCII(key) {
		var buf [128]byte
		n := normalizeASCII(key, buf[:])
		if _, ok := h.keys[string(buf[:n])]; ok {
			return true
		}
		return matchSegmentsASCII(h.keys, key)
	}
	if _, ok := h.keys[normalize(key)]; ok {
		return true
	}
	segs := segments(key)
	for _, s := range segs {
		if _, ok := h.keys[s]; ok {
			return true
		}
	}
	for i := 0; i < len(segs)-1; i++ {
		if _, ok := h.keys[segs[i]+segs[i+1]]; ok {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func normalizeASCII(key string, buf []byte) int {
	n := 0
	for i := 0; i < len(key) && n < len(buf); i++ {
		c := key[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			buf[n] = c
			n++
		}
	}
	return n
}

func matchSegmentsASCII(keys map[string]struct{}, key string) bool {
	var seg [64]byte
	var prev [64]byte
	segN, prevN := 0, 0
	hasPrev := false
	flush := func() bool {
		if segN == 0 {
			return false
		}
		if _, ok := keys[string(seg[:segN])]; ok {
			return true
		}
		if hasPrev {
			var join [128]byte
			copy(join[:], prev[:prevN])
			copy(join[prevN:], seg[:segN])
			if _, ok := keys[string(join[:prevN+segN])]; ok {
				return true
			}
		}
		copy(prev[:], seg[:segN])
		prevN = segN
		hasPrev = true
		segN = 0
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		isUpper := c >= 'A' && c <= 'Z'
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		if isUpper && segN > 0 {
			prevByte := key[i-1]
			prevLower := prevByte >= 'a' && prevByte <= 'z'
			nextLower := i+1 < len(key) && key[i+1] >= 'a' && key[i+1] <= 'z'
			prevUpper := prevByte >= 'A' && prevByte <= 'Z'
			if prevLower || (prevUpper && nextLower) {
				if flush() {
					return true
				}
			}
		}
		if isUpper {
			c += 'a' - 'A'
		}
		if isUpper || isLower || isDigit {
			if segN < len(seg) {
				seg[segN] = c
				segN++
			}
			continue
		}
		if flush() {
			return true
		}
	}
	return flush()
}

func isSecret(v any) bool {
	_, ok := v.(Secret)
	return ok
}

func normalize(k string) string {
	var b strings.Builder
	b.Grow(len(k))
	for _, r := range k {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func segments(key string) []string {
	runes := []rune(key)
	var segs []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		segs = append(segs, b.String())
		b.Reset()
	}
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if i > 0 && unicode.IsUpper(r) && b.Len() > 0 {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || (unicode.IsUpper(prev) && nextLower) {
				flush()
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	flush()
	return segs
}
