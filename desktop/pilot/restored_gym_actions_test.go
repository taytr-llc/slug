package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
)

func TestRestoredRecoveryDiscoveryResultOwnership(t *testing.T) {
	candidate := fighterownership.RecoveryCommitCandidate{CommitTxID: restoredActionTestID(1), ProspectID: restoredActionTestID(2), Height: 123}
	d := restoredDiscovery{candidate: &candidate, pageComplete: true}
	v, e := d.finish("currently_absent", 123456)
	if e != nil {
		t.Fatal(e)
	}
	v.Candidate.CommitTxID = restoredActionTestID(3)
	v.Status = "matched"
	v.ObservedAtMS = 1
	if d.last.Candidate != candidate || d.last.Status != "currently_absent" || d.last.ObservedAtMS != 123456 || d.cursor != 1 || !d.complete {
		t.Fatal("public mutation changed retained dated result")
	}
	if _, e = d.finish("matched", 123457); e == nil {
		t.Fatal("same candidate completed twice")
	}
}
func TestRestoredActionProjectionDoesNotExposeSignedBytes(t *testing.T) {
	v := fightercarrier.Operation{RequestID: restoredActionTestID(1), Request: fightercarrier.Request{Kind: "recover_reveal", CommitTxID: restoredActionTestID(2)}, Stage: "signed", SignedPackage: &fightercarrier.SignedPackage{TransferID: restoredActionTestID(3), Commit: fightercarrier.RawTransaction{Raw: "private signed commit", TxID: restoredActionTestID(2)}, Spend: fightercarrier.RawTransaction{Raw: "private signed spend", TxID: restoredActionTestID(4)}}, CommitObservation: &fightercarrier.Observation{State: "canonical", BlockHeight: 9}}
	out := restoredActionStatus(v)
	out.CommitObservation.BlockHeight = 10
	if v.CommitObservation.BlockHeight != 9 {
		t.Fatal("projection aliased saved observation")
	}
	raw, e := json.Marshal(out)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		t.Fatal("projection")
	}
	for _, name := range []string{"raw", "signed_package", "gym_authorization", "preparation"} {
		if _, ok := fields[name]; ok {
			t.Fatal("secret/raw field exposed")
		}
	}
	if len(fields) != 7 || out.CommitTxID != v.Request.CommitTxID {
		t.Fatal("public projection shape")
	}
}
func TestRestoredDeliveryReviewOneUseBeforeAnyNodeAccess(t *testing.T) {
	if !restoredSupported() {
		t.Skip("supported platform")
	}
	path, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	root, e := openRestoredRoot(filepath.Join(path, "owner"), true)
	if e != nil {
		t.Fatal(e)
	}
	life, cancel := context.WithCancel(context.Background())
	s := &RestoredGymSession{life: life, cancel: cancel, gate: make(chan struct{}, 1), root: root}
	defer func() {
		if e := s.Close(); e != nil {
			t.Fatal(e)
		}
	}()
	s.deliveryReview = &RestoredDeliveryReview{ReviewID: restoredActionTestID(1), ExpiresAtMS: time.Now().Add(time.Minute).UnixMilli()}
	if _, e = s.Deliver(context.Background(), restoredActionTestID(2)); !errors.Is(e, fightercarrier.ErrUnauthorized) {
		t.Fatal("wrong review did not refuse before absent node", e)
	}
	if s.deliveryReview != nil {
		t.Fatal("review was reusable")
	}
	if _, e = s.Deliver(context.Background(), restoredActionTestID(1)); !errors.Is(e, fightercarrier.ErrUnauthorized) {
		t.Fatal("wrong review restored authority", e)
	}
	s.deliveryReview = &RestoredDeliveryReview{ReviewID: restoredActionTestID(1), ExpiresAtMS: time.Now().Add(-time.Second).UnixMilli()}
	if _, e = s.Deliver(context.Background(), restoredActionTestID(1)); !errors.Is(e, fightercarrier.ErrUnauthorized) {
		t.Fatal("expired review accepted", e)
	}
}

func restoredActionTestID(n int) string { return fmt.Sprintf("%064x", n) }
