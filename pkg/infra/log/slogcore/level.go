package slogcore

import (
	"log/slog"
	"strings"
)

// ParseLevel maps a Grafana level name to an slog level. "trace" maps to debug and
// "critical" to error, matching the legacy engine.
func ParseLevel(name string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "trace", "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error", "critical":
		return slog.LevelError, true
	default:
		return slog.LevelError, false
	}
}

// ParseFilters parses `logger:level` entries such as `sqlstore:debug`. Entries starting
// with `;` or `#` are comments; a bare comment marker also skips the next entry, matching
// the legacy parser. Unknown levels fall back to error and are reported in invalid.
func ParseFilters(entries []string) (filters map[string]slog.Level, invalid []string) {
	filters = make(map[string]slog.Level)
	for i := 0; i < len(entries); i++ {
		entry := strings.TrimSpace(entries[i])
		if strings.HasPrefix(entry, ";") || strings.HasPrefix(entry, "#") {
			if len(entry) == 1 {
				i++
			}
			continue
		}
		name, levelName, ok := strings.Cut(entry, ":")
		if !ok {
			continue
		}
		levelName, _, _ = strings.Cut(levelName, ":")
		lvl, ok := ParseLevel(levelName)
		if !ok {
			invalid = append(invalid, levelName)
		}
		filters[name] = lvl
	}
	return filters, invalid
}

func levelLabel(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}
