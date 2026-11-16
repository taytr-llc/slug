package pilot

import "testing"

func TestAccountProtocolAcceptsOnlyFixtureLabels(t *testing.T) {
	for _, raw := range []string{
		`{"id":1,"command":"login_fixture","fixture":"zero256"}`,
		`{"id":2,"command":"login_fixture","fixture":"incrementing256"}`,
		`{"id":3,"command":"logout"}`,
		`{"id":4,"command":"reset_fixture"}`,
	} {
		if _, err := parseRequest([]byte(raw)); err != nil {
			t.Fatalf("fixture request rejected: %v", err)
		}
	}
	for _, raw := range []string{
		`{"id":0,"command":"login_fixture","fixture":"zero256"}`,
		`{"id":1,"command":"login_fixture"}`,
		`{"id":1,"command":"login_fixture","fixture":"real-wallet"}`,
		`{"id":1,"command":"login_fixture","fixture":"zero256","subject":"other"}`,
		`{"id":1,"command":"login_fixture","fixture":"zero256","issuer":"other"}`,
		`{"id":1,"command":"reset_fixture","verified":true}`,
		`{"id":1,"command":"reset_fixture","hold_until_ms":0}`,
		`{"id":1,"command":"reset_fixture","fixture":"zero256"}`,
		`{"id":1,"command":"login_fixture","fixture":"zero256","token":"secret"}`,
		`{"id":1,"command":"login_fixture","fixture":"zero256","fixture":"incrementing256"}`,
		`{"id":1,"command":"logout","path":"/tmp/recovery"}`,
	} {
		if _, err := parseRequest([]byte(raw)); err == nil {
			t.Fatal("accepted caller-chosen account authority")
		}
	}
}
