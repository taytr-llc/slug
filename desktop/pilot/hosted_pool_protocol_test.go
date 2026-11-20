package pilot

import (
	"encoding/json"
	"testing"
)

func TestHostedPoolSnapshotClosedModeAndPendingReset(t *testing.T) {
	s := BlankPoolSnapshot()
	s.Mode = HostedPoolMode
	s.Account.Fixture = "zero256"
	s.Account.LastError = "hosted_reset_pending"
	raw, err := json.Marshal(PoolResponse{ID: 1, OK: true, Snapshot: &s})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParsePoolResponse(raw); err != nil {
		t.Fatal("bound signed-out pending reset refused", err)
	}
	for _, change := range []func(*PoolSnapshot){
		func(v *PoolSnapshot) { v.Mode = PoolMode },
		func(v *PoolSnapshot) { v.Mode = "hosted" },
		func(v *PoolSnapshot) { v.Account.Fixture = "" },
		func(v *PoolSnapshot) { v.Account.State = "resetting" },
		func(v *PoolSnapshot) { v.Account.CanExport = true },
		func(v *PoolSnapshot) {
			v.Account.ID = "1111111111111111111111111111111111111111111111111111111111111111"
		},
	} {
		bad := s
		change(&bad)
		if ValidatePoolSnapshot(bad) == nil {
			t.Fatal("invalid pending reset state accepted")
		}
	}
	s.Account.LastError = ""
	s.Account.Fixture = ""
	if ValidatePoolSnapshot(s) != nil {
		t.Fatal("ordinary hosted signed-out state refused")
	}
}
