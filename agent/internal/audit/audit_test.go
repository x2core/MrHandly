package audit

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLogEmitsJSON(t *testing.T) {
	var buf bytes.Buffer
	a := New(&buf)
	a.Log("10.44.0.9", "request", "GET /v1/info", ResultForbidden)

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("audit line is not JSON: %v\n%s", err, buf.String())
	}
	for k, want := range map[string]string{
		"actor":  "10.44.0.9",
		"action": "request",
		"target": "GET /v1/info",
		"result": ResultForbidden,
		"msg":    "audit",
	} {
		if got, _ := rec[k].(string); got != want {
			t.Errorf("field %q = %v, want %q", k, rec[k], want)
		}
	}
}
