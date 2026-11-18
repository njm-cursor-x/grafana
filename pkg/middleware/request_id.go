package middleware

import (
	"net/http"
	"unicode"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/grafana/grafana/pkg/infra/log/logctx"
	"github.com/grafana/grafana/pkg/infra/log/logkeys"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/grafana/grafana/pkg/web"
)

// RequestIDHeader is the HTTP header used to echo (and optionally accept) a request id.
const RequestIDHeader = "X-Request-Id"

const maxRequestIDLength = 128

// RequestID returns middleware that assigns a request id, stores it on the
// request context, echoes it on the response, and records it on the active span.
// Incoming X-Request-Id values are used only when cfg.TrustIncomingRequestID is true
// and the value is a well-formed id; otherwise a new id is generated.
func RequestID(cfg *setting.Cfg) web.Middleware {
	trustIncoming := cfg != nil && cfg.TrustIncomingRequestID
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id := ""
			if trustIncoming {
				id = sanitizeRequestID(req.Header.Get(RequestIDHeader))
			}
			if id == "" {
				id = uuid.NewString()
			}

			ctx := logctx.WithRequestID(req.Context(), id)
			w.Header().Set(RequestIDHeader, id)
			trace.SpanFromContext(ctx).SetAttributes(attribute.String(logkeys.KeyRequestID, id))

			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

func sanitizeRequestID(id string) string {
	if id == "" || len(id) > maxRequestIDLength {
		return ""
	}
	for _, r := range id {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.') {
			return ""
		}
	}
	return id
}
