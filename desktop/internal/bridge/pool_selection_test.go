// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/wire/v2"
	"strings"
	"testing"
)

func TestPoolBridgePinsFirstDescriptorToLaunchOptions(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal source identity")
	}
	v1, e := pilot.ResolvePoolOptions(pilot.PoolOptions{})
	if e != nil {
		t.Fatal(e)
	}
	v2, e := fighterdescriptor.ID(fighterdescriptor.CandidateNameV2Descriptor())
	if e != nil {
		t.Fatal(e)
	}
	for _, selected := range []string{v1.CandidateDescriptorID, v2} {
		b := &PoolBridge{&StableBridge{poolExpectedDescriptor: selected}}
		s := poolBridgeSnapshot(t)
		s.View.AuthorityPin.DescriptorID = selected
		if !b.matchesPoolSelection(&s) {
			t.Fatal("matching initial descriptor refused")
		}
		other := v2
		if selected == v2 {
			other = v1.CandidateDescriptorID
		}
		s.View.AuthorityPin.DescriptorID = other
		if b.matchesPoolSelection(&s) {
			t.Fatal("first disclosure auto-selected a different supported descriptor")
		}
		s.View.AuthorityPin.DescriptorID = selected
		b.poolSourceID = s.SourceBindingID
		p := s.View.AuthorityPin
		b.poolSourcePin = &p
		if !b.matchesPoolSelection(&s) {
			t.Fatal("same source refused")
		}
		s.View.AuthorityPin.AuthorityID = strings.Repeat("e", 64)
		if b.matchesPoolSelection(&s) {
			t.Fatal("descriptor match replaced issuer")
		}
	}
	if _, e := LaunchPoolWithOptions("/not-a-helper", "/tmp/not-created", "/not-a-node", pilot.PoolOptions{CandidateDescriptorID: "unknown"}); e == nil {
		t.Fatal("unknown launch choice")
	}
}
