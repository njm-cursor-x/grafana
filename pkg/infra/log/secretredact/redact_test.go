package secretredact

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const leakProbe = "test-secret"

func TestRedactSecrets_BearerAuthorization(t *testing.T) {
	got := RedactSecrets("Authorization: Bearer " + leakProbe)
	if strings.Contains(got, leakProbe) {
		t.Fatalf("secret leaked in %q", got)
	}
	if !strings.Contains(got, Redacted) {
		t.Fatalf("expected %s in %q", Redacted, got)
	}
}

func TestBearerTokenDoesNotAppearInEncodedLogOutput(t *testing.T) {
	keyvals := []any{
		"msg", "request failed",
		"Authorization", "Bearer " + leakProbe,
		"cookie", "grafana_session=" + leakProbe,
		"err", fmt.Errorf("upstream rejected Authorization: Bearer %s", leakProbe),
		"headers", http.Header{"Authorization": []string{"Bearer " + leakProbe}},
	}
	redacted := RedactLogKeyvals(keyvals)
	encoded, err := json.Marshal(mapFromKeyvals(redacted))
	if err != nil {
		t.Fatal(err)
	}
	out := string(encoded)
	if out == "" {
		t.Fatal("empty encoded output")
	}
	if strings.Contains(out, leakProbe) {
		t.Fatalf("probe secret leaked in encoded log output: %s", out)
	}
	if !strings.Contains(out, "request failed") {
		t.Fatalf("safe message missing from %s", out)
	}
	if !strings.Contains(out, Redacted) {
		t.Fatalf("expected %s in %s", Redacted, out)
	}
}

func TestRedactSecrets_IDTokenAssignment(t *testing.T) {
	// Probe avoids the substring "secret" so maybeContainsSecret must match id_token itself.
	probe := "idtok-aabbccdd1122"
	got := RedactSecrets("oauth failed id_token=" + probe)
	if strings.Contains(got, probe) {
		t.Fatalf("id_token leaked in %q", got)
	}
	if got != "oauth failed id_token="+Redacted {
		t.Fatalf("got %q", got)
	}
}

func TestUserControlledStringsAreFieldsNotFormatSinks(t *testing.T) {
	title := `Ops %s %!(EXTRA string=boom) %d`
	query := `up{job='%s'} OR rate(http_requests[5m])`
	redacted := RedactLogKeyvals([]any{
		"msg", "query failed",
		"dashboardTitle", title,
		"query", query,
	})
	encoded, err := json.Marshal(mapFromKeyvals(redacted))
	if err != nil {
		t.Fatal(err)
	}
	out := string(encoded)
	if !strings.Contains(out, title) {
		t.Fatalf("dashboard title was not preserved as a field: %s", out)
	}
	if !strings.Contains(out, query) {
		t.Fatalf("query text was not preserved as a field: %s", out)
	}
}

func TestRedactSecrets_BasicAuth(t *testing.T) {
	got := RedactSecrets("Authorization: Basic dXNlcjpwYXNz")
	if strings.Contains(got, "dXNlcjpwYXNz") {
		t.Fatalf("basic secret leaked in %q", got)
	}
	if got != "Authorization: "+Redacted {
		t.Fatalf("got %q", got)
	}
}

func TestRedactValue_FragmentKeys(t *testing.T) {
	if got := RedactValue("sessionToken", leakProbe); got != Redacted {
		t.Fatalf("sessionToken: got %#v", got)
	}
	if got := RedactValue("clientCredential", leakProbe); got != Redacted {
		t.Fatalf("clientCredential: got %#v", got)
	}
	if got := RedactValue("dashboardTitle", "Sales %s"); got != "Sales %s" {
		t.Fatalf("safe key was redacted: %#v", got)
	}
}

func TestRedactLogKeyvals_OddTrailingValue(t *testing.T) {
	got := RedactLogKeyvals([]any{"orphan Bearer " + leakProbe})
	if len(got) != 1 {
		t.Fatalf("len %d", len(got))
	}
	s, ok := got[0].(string)
	if !ok {
		t.Fatalf("got %T", got[0])
	}
	if strings.Contains(s, leakProbe) {
		t.Fatalf("leaked in %q", s)
	}
	if !strings.Contains(s, "Bearer "+Redacted) {
		t.Fatalf("got %q", s)
	}
}

func TestRedactLogKeyvalsLeavesSafeFields(t *testing.T) {
	in := []any{"msg", "hello", "dashboardTitle", "Sales %s", "orgId", 1}
	out := RedactLogKeyvals(in)
	if len(out) != len(in) {
		t.Fatalf("len %d != %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("arg %d changed: %#v -> %#v", i, in[i], out[i])
		}
	}
	out[1] = "mutated"
	if in[1] != "hello" {
		t.Fatal("RedactLogKeyvals aliased the caller slice")
	}
}

func mapFromKeyvals(keyvals []any) map[string]any {
	m := make(map[string]any, (len(keyvals)+1)/2)
	for i := 0; i+1 < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			key = fmt.Sprint(keyvals[i])
		}
		m[key] = keyvals[i+1]
	}
	return m
}
