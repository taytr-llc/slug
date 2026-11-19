// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

const nameV2DescriptorFixture = "cd02ae57b5918548244be2cabbe6c0c433feb5a1c165745e2e5c6bf235128e75"
const nameV1DescriptorFixture = "c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303"

type nameV2CardCase struct {
	Seed           string
	Manifest       fightermanifest.Manifest
	ManifestDigest string `json:"manifest_digest"`
	PortraitDigest string `json:"svg_digest"`
}

func nameV2CardCases(t *testing.T) []nameV2CardCase {
	t.Helper()
	raw, err := os.ReadFile("../fighterdescriptor/testdata/name-v2/independent/reference-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "62c4cd102341476548b9d388437af83750be407b8cbf534b34896719e2fa281b" {
		t.Fatal("independent v2 corpus changed")
	}
	var ref struct{ Cases []nameV2CardCase }
	if err := json.Unmarshal(raw, &ref); err != nil || len(ref.Cases) != 764 {
		t.Fatal("independent corpus shape", err)
	}
	return ref.Cases
}

func nameV2CardCorner(t *testing.T, f nameV2CardCase) (Pin, BoutCorner) {
	t.Helper()
	// New synthetic inputs for private derivation boundaries only. These are
	// not a purported source receipt, chain finality, or accepted fighter.
	p := Pin{Version: 1, RulesID: RulesID()}
	for _, ch := range []string{"a", "b"} {
		s := gymstable.Pin{AccountAuthorityID: strings.Repeat("c", 64), AccountID: strings.Repeat(ch, 64)}
		s.Identity.GymID = strings.Repeat(ch, 64)
		s.Source.DescriptorID = nameV2DescriptorFixture
		p.SourcePin.SourcePin.Participants = append(p.SourcePin.SourcePin.Participants, gymentry.Pin{StablePin: s})
	}
	c, err := fightercondition.Project(1, f.Manifest.DNA, 1, 4, 0)
	if err != nil {
		t.Fatal(err)
	}
	a := gymstable.Assignment{Version: 1, Sequence: 1, ProspectID: strings.Repeat("d", 64), GymID: strings.Repeat("a", 64), WorkHash: f.Seed, BornAtMS: 1, AcceptedAtMS: 2, DescriptorID: nameV2DescriptorFixture, ManifestDigest: f.ManifestDigest, PortraitDigest: f.PortraitDigest, SourceRecordDigest: strings.Repeat("e", 64)}
	return p, BoutCorner{
		Owner:  gymevent.AssignmentOwner{OwnershipBasis: "candidate_assignment", AccountAuthorityID: strings.Repeat("c", 64), AccountID: strings.Repeat("a", 64), GymID: a.GymID},
		Member: gymevent.Member{Slot: 1, SlotAssignedAtMS: 3, Assignment: a, Class: f.Manifest.Physical.Class, ConditionAtCutoff: c, Eligibility: "eligible"},
	}
}

func TestNameV2CardCornerKeepsExactParticipant(t *testing.T) {
	for n, f := range nameV2CardCases(t) {
		if n > 2 && !strings.Contains(f.Manifest.Name.Display, "'") {
			continue
		}
		p, corner := nameV2CardCorner(t, f)
		if err := validateBoutCorner(p, corner); err != nil {
			t.Fatalf("v2 corner %d: %v", n, err)
		}
		for _, change := range []func(*BoutCorner){
			func(c *BoutCorner) { c.Member.Assignment.DescriptorID = nameV1DescriptorFixture },
			func(c *BoutCorner) { c.Member.Assignment.DescriptorID = strings.Repeat("f", 64) },
			func(c *BoutCorner) { c.Member.Assignment.ManifestDigest = strings.Repeat("f", 64) },
			func(c *BoutCorner) { c.Owner.AccountID = strings.Repeat("f", 64) },
			func(c *BoutCorner) { c.Member.Assignment.GymID = strings.Repeat("f", 64) },
			func(c *BoutCorner) { c.Member.Class = "wrong" },
		} {
			bad := corner
			change(&bad)
			if err := validateBoutCorner(p, bad); err == nil {
				t.Fatal("foreign descriptor/owner or altered manifest accepted")
			}
		}
	}
}

func TestNameV2CardEntrantUsesIndependentName(t *testing.T) {
	f := nameV2CardCases(t)[0]
	p, corner := nameV2CardCorner(t, f)
	s := gymseed.CardSource{EventKey: strings.Repeat("1", 64), Target: gymseed.Target{Height: 1, Hash: strings.Repeat("2", 64)}}
	s.SourceReceipt.Input.Event.EventID = strings.Repeat("3", 64)
	s.SourceReceipt.SnapshotDigest = strings.Repeat("4", 64)
	s.SourceReceipt.Input.Participants = []gymevent.ParticipantSnapshot{{Owner: corner.Owner, Members: []gymevent.Member{corner.Member}}}
	// Deliberately the private already-validated derivation boundary. No claim
	// that the synthetic source above has passed public finality validation.
	plan, err := derivePlanValidated(p, s)
	if err != nil || len(plan.Entrants) != 1 {
		t.Fatal("v2 entrant derivation", err)
	}
	e := plan.Entrants[0]
	if e.DisplayName != f.Manifest.Name.Display || e.Class != f.Manifest.Physical.Class || e.ProspectID != corner.Member.Assignment.ProspectID || e.RatingHundredths != uint32(corner.Member.ConditionAtCutoff.RatingHundredths) || e.State != "unmatched" || e.Reason != "no_opponent_in_class" {
		t.Fatal("name dispatch changed non-name card semantics")
	}
	s.SourceReceipt.Input.Participants[0].Members[0].Assignment.DescriptorID = nameV1DescriptorFixture
	if _, err = derivePlanValidated(p, s); err == nil {
		t.Fatal("v1 manifest substituted for independent v2 digest")
	}
}
