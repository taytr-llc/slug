// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCarrierIdentityInheritedJSONAndBinding(t *testing.T) {
	count := 0
	for _, row := range readWalletReference(t).Positives {
		if row.Kind != "journal_marker" {
			continue
		}
		var marker journalMarker
		if e := json.Unmarshal(row.Value, &marker); e != nil {
			t.Fatal(e)
		}
		identity := Identity{JournalID: marker.JournalID, Pin: marker.Pin}
		// This untagged shape is the pre-fix encoding contract. Explicit tags must
		// preserve its exact bytes, including the inherited uppercase field names.
		old := struct {
			JournalID string
			Pin       Pin
		}{identity.JournalID, identity.Pin}
		oldRaw, e := json.Marshal(old)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := json.Marshal(identity)
		if e != nil || !bytes.Equal(raw, oldRaw) {
			t.Fatal("changed inherited Identity bytes", row.Name, e)
		}
		if !bytes.HasPrefix(raw, []byte(`{"JournalID":`)) || !bytes.Contains(raw, []byte(`,"Pin":`)) {
			t.Fatal("changed inherited field spelling")
		}
		got, e := BindingID(identity)
		if e != nil || got != marker.BindingID {
			t.Fatal("changed independent binding digest", row.Name, e)
		}
		count++
	}
	if count == 0 {
		t.Fatal("no independently frozen journal marker inputs")
	}
}
