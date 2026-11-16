package pilot

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecoveryRequestBoundary(t *testing.T) {
	for _, command := range []string{"export_recovery", "verify_backup"} {
		raw, err := json.Marshal(Request{ID: 1, Command: command, Path: "/private/tmp/test wallet.json"})
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parseRequest(raw)
		if err != nil || parsed.Path != "/private/tmp/test wallet.json" {
			t.Fatalf("native selection: %v", err)
		}
		for _, path := range []string{"", "relative.json", "/tmp/a\nb", "/tmp/a\x00b", "/" + strings.Repeat("x", 1024)} {
			raw, _ := json.Marshal(Request{ID: 1, Command: command, Path: path})
			if _, err := parseRequest(raw); err == nil {
				t.Fatal("accepted invalid native path")
			}
		}
	}
	for _, raw := range []string{
		`{"id":1,"command":"snapshot","path":"/tmp/a"}`,
		`{"id":1,"command":"receipt","sequence":1,"path":"/tmp/a"}`,
		`{"id":1,"command":"export_recovery","path":"/tmp/a","sequence":1}`,
		`{"id":1,"command":"verify_backup","path":"/tmp/a","path":"/tmp/b"}`,
		`{"id":1,"command":"export_recovery","path":null}`,
		`{"id":1,"command":"verify_backup","path":"/tmp/a","mnemonic":"private"}`,
	} {
		if _, err := parseRequest([]byte(raw)); err == nil {
			t.Fatal("accepted malformed recovery request")
		}
	}
	raw := []byte(`{"id":1,"command":"verify_backup","path":"/tmp/x"}`)
	raw[len(raw)-4] = 0xff
	if _, err := parseRequest(raw); err == nil {
		t.Fatal("accepted invalid UTF8")
	}
}
