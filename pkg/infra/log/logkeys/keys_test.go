package logkeys

import "testing"

func TestCanonicalKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{in: "error", want: KeyError, wantOK: true},
		{in: "err", want: KeyError, wantOK: true},
		{in: "orgId", want: KeyOrgID, wantOK: true},
		{in: "orgID", want: KeyOrgID, wantOK: true},
		{in: KeyOrgID, want: KeyOrgID, wantOK: true},
		{in: "userId", want: KeyUserID, wantOK: true},
		{in: "userID", want: KeyUserID, wantOK: true},
		{in: "traceID", want: KeyTraceID, wantOK: true},
		{in: "traceId", want: KeyTraceID, wantOK: true},
		{in: "spanID", want: KeySpanID, wantOK: true},
		{in: "requestID", want: KeyRequestID, wantOK: true},
		{in: "logger", want: KeyLogger, wantOK: true},
		{in: "namespace", want: KeyNamespace, wantOK: true},
		{in: "uname", wantOK: false},
		{in: "username", wantOK: false},
		{in: "login", wantOK: false},
		{in: "user.name", wantOK: false},
		{in: "dashboardUID", want: "dashboardUID", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, ok := CanonicalKey(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("CanonicalKey(%q) ok=%v, want %v", tt.in, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("CanonicalKey(%q)=%q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
