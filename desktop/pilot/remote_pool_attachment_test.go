// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestRemotePoolNodeSelectionIsPrivateLocalOnly(t *testing.T) {
	valid := prospectnode.RPCConfig{Version: 1, URL: "http://127.0.0.1:12345", Username: "published-test-user", Password: "published-test-password"}
	if remotePoolValidateNode(valid) != nil {
		t.Fatal("existing HTTP loopback boundary refused")
	}
	for _, url := range []string{"https://127.0.0.1:12345", "http://localhost:12345", "http://[::1]:12345", "http://127.0.0.1", "http://127.0.0.1:0", "http://127.0.0.1:65536", "http://127.0.0.1:0123", "http://127.0.0.1:12345/", "http://127.0.0.1:12345?", "http://127.0.0.1:12345#x", "http://u:p@127.0.0.1:12345", "http://192.0.2.1:12345"} {
		c := valid
		c.URL = url
		if remotePoolValidateNode(c) == nil {
			t.Fatal("unsupported node selection admitted", url)
		}
	}
	valid.CACertificate = "public but forbidden at this local boundary"
	if remotePoolValidateNode(valid) == nil {
		t.Fatal("wallet transport widened by CA")
	}
}

func TestRemotePoolEstablishedRecipientControlsBeforeOpen(t *testing.T) {
	// Public metadata-only audit; no fabricated wallet READY authority or node.
	b := remotePoolTestReference(t).Positives[0].Config.ExpectedLocal.Recipient
	root := t.TempDir()
	ready := &ownedWalletReady{Recipient: b}
	paths := []struct{ name, phase string }{{poolRecipientIntentName, "initializing"}, {poolRecipientReadyName, "ready"}}
	raw := map[string][]byte{}
	for _, p := range paths {
		v, e := json.Marshal(recipientMarker(b, p.phase))
		if e != nil {
			t.Fatal(e)
		}
		raw[p.name] = append(v, '\n')
		if e = os.WriteFile(filepath.Join(root, p.name), raw[p.name], 0600); e != nil {
			t.Fatal(e)
		}
	}
	if e := auditOwnedWalletRecipientControls(root, ready); e != nil {
		t.Fatal("matching established controls", e)
	}
	for _, p := range paths {
		path := filepath.Join(root, p.name)
		if e := os.Remove(path); e != nil {
			t.Fatal(e)
		}
		if auditOwnedWalletRecipientControls(root, ready) == nil {
			t.Fatal("missing established control admitted")
		}
		if _, e := os.Lstat(path); !os.IsNotExist(e) {
			t.Fatal("pre-open audit repaired missing control", e)
		}
		bad := append([]byte(nil), raw[p.name]...)
		bad[len(bad)-1] = ' '
		if e := os.WriteFile(path, bad, 0600); e != nil {
			t.Fatal(e)
		}
		if auditOwnedWalletRecipientControls(root, ready) == nil {
			t.Fatal("noncanonical established control admitted")
		}
		after, e := os.ReadFile(path)
		if e != nil || !bytes.Equal(after, bad) {
			t.Fatal("pre-open audit rewrote control", e)
		}
		if e = os.WriteFile(path, raw[p.name], 0600); e != nil {
			t.Fatal(e)
		}
	}
	other := *ready
	other.Recipient.Config.AccountID = other.Recipient.Config.AccountAuthorityID
	if auditOwnedWalletRecipientControls(root, &other) == nil {
		t.Fatal("replacement recipient adopted")
	}
}

func TestRemotePoolAttachmentCheapRefusalBeforeEffects(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("refusal profile only")
	}
	root := filepath.Join(t.TempDir(), "must-not-exist")
	a, e := OpenRemotePoolAttachment(context.Background(), nil, "", RemotePoolInputs{AttachmentRoot: root})
	if a != nil || !errors.Is(e, ErrRemotePoolProfile) {
		t.Fatal("cheap profile reached construction", e)
	}
	if _, e = os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("cheap profile wrote attachment", e)
	}
}
func TestRemotePoolAttachmentZeroAndPrivateStatus(t *testing.T) {
	var a RemotePoolAttachment
	if a.Close() == nil {
		t.Fatal("zero owner advertised clean original resources")
	}
	if e := a.Start(t.Context(), "foreign"); e == nil {
		t.Fatal("zero owner admitted start")
	}
	if _, e := a.Snapshot(t.Context(), "foreign"); e == nil {
		t.Fatal("zero owner disclosed snapshot")
	}
	if remotePoolPublicCode("private-config-credential-detail") != "unavailable" || remotePoolWorkerState("private-error") != "unavailable" {
		t.Fatal("raw worker detail leaked")
	}
	if remotePoolPublicCode("capacity") != "capacity" || remotePoolWorkerState("paused") != "paused" {
		t.Fatal("capacity state concealed")
	}
}

func TestRemotePoolSavedAdmissionEmptyBranch(t *testing.T) {
	c := remotePoolTestReference(t).Positives[0].Config
	key := c.ExpectedLocal.Recipient.Config.Identity.GymPublicKey
	pin := c.Source.AuthorityPin
	// This exercises only selection after the real outbox's audit; these small
	// shaped values do not claim a mined proof or replace canonical validation.
	initial := prospect.Context{Version: c.Source.Policy.Version, Network: c.Source.Policy.Network, Genesis: c.Source.Policy.Genesis, DNAVersion: c.Source.Policy.DNAVersion, ProspectTarget: c.Source.Policy.ProspectTarget}
	proof := prospect.Proof{Version: prospect.Version2, GymID: c.ExpectedLocal.Recipient.Config.Identity.GymID, MerkleBranch: []string{}}
	row := prospectworker.ManifestEntry{ProspectWin: true, AdmissionState: "needs-signature", Job: prospectjob.ManifestJob{Context: initial, PublicKey: key, AuthorityID: pin.AuthorityID, DescriptorID: pin.DescriptorID}, Candidate: prospectwork.Candidate{Proof: proof}}
	owned := remotePoolOwnAdmissionProof(proof)
	if owned.MerkleBranch == nil || !remotePoolSavedAdmissionMatches(&row, initial, owned, key, pin) {
		t.Fatal("canonical empty branch lost its exact saved match")
	}
	row.Job.AuthorityID = c.ConnectionID
	if remotePoolSavedAdmissionMatches(&row, initial, owned, key, pin) {
		t.Fatal("replacement authority matched saved work")
	}
	row.Job.AuthorityID = pin.AuthorityID
	row.AdmissionState = "accepted"
	if remotePoolSavedAdmissionMatches(&row, initial, owned, key, pin) {
		t.Fatal("already accepted row granted new signature")
	}
	proof.MerkleBranch = []string{c.ConnectionID}
	owned = remotePoolOwnAdmissionProof(proof)
	owned.MerkleBranch[0] = "changed"
	if proof.MerkleBranch[0] != c.ConnectionID {
		t.Fatal("owned proof aliased original row")
	}
}

func TestRemotePoolAuditFailedCloserRetained(t *testing.T) {
	f, e := os.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	failure := remotePoolCloseAuditFile(f)
	var held *remotePoolAuditCloseError
	if !errors.Is(failure, ErrRemotePoolCleanup) || !errors.As(failure, &held) || held.file != f {
		t.Fatal("lost exact failed audit descriptor", failure)
	}
	// No raw descriptor retry: the returned immutable error is the retained owner.
	if held.Error() != "remote pool audit cleanup incomplete" {
		t.Fatal("private path exposed")
	}
}
