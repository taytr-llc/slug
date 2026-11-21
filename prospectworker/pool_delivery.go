// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
)

func (worker *PoolWorker) deliveryAuthoritiesMatch() bool {
	a, f, c := worker.authority, worker.admission, worker.config
	return a != nil && f != nil && a.ProtocolVersion() == c.Version && a.AuthorityOrigin() == c.AuthorityOrigin && a.SourceConfig() == c.Source && a.TransportTrustID() == c.TransportTrustID && a.PublicKey() == c.PublicKey && f.ProtocolVersion() == 3 && f.AuthorityOrigin() == c.AdmissionOrigin && f.AuthorityPin() == c.Source.AuthorityPin && f.TransportTrustID() == c.AdmissionTrustID
}

// deliver treats pool credit, block publication and fighter admission as separate
// outcomes. All recover the exact durable work; none can issue a replacement job.
func (worker *PoolWorker) deliver(ctx context.Context, entry *PoolEntry) (bool, error) {
	if entry == nil || validatePoolEntry(*entry, worker.config) != nil || !worker.deliveryAuthoritiesMatch() {
		return false, ErrCorrupt
	}
	prepared, err := preparePoolWork(worker.config, entry.Work)
	if err != nil {
		return false, ErrCorrupt
	}
	// Disk classifications are claims until this exact nonce is hashed locally.
	check, err := prepared.CheckNonce(ctx, entry.Nonce)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, prospectwork.ErrShareBusy) {
			return false, nil
		}
		return false, err
	}
	expected, err := poolCheck(*entry, worker.config)
	if err != nil || !reflect.DeepEqual(check, expected) {
		return false, ErrCorrupt
	}
	if ctx.Err() != nil {
		return false, nil
	}
	progress := false
	fighter := entry.Fighter
	if fighter != nil && fighter.BlockWin && fighter.BlockState != "canonical" && fighter.BlockState != "rejected" {
		call, cancel := context.WithTimeout(ctx, 30*time.Second)
		// D69: synchronize first, broadcast before pool recovery or any signer.
		err := worker.node.SubmitBlock(call, fighter.Job.Context, fighter.Candidate.BlockHex)
		cancel()
		fighter.BlockState = "submitted"
		fighter.LastError = ""
		if err != nil {
			var failure *prospectnode.SubmissionError
			if errors.As(err, &failure) {
				fighter.LastError = failure.Code
			} else {
				fighter.LastError = "outcome_unknown"
			}
		}
		if err := worker.outbox.update(*entry); err != nil {
			return false, err
		}
		if ctx.Err() != nil {
			return false, nil
		}
		call, cancel = context.WithTimeout(ctx, 30*time.Second)
		proof := fighter.Candidate.Proof
		proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
		err = worker.node.CanonicalBlock(call, fighter.Job.Context, proof, fighter.WorkHash)
		cancel()
		if err == nil {
			fighter.BlockState, fighter.LastError = "canonical", ""
			progress = true
			if err := worker.outbox.update(*entry); err != nil {
				return false, err
			}
			if worker.current != nil && worker.current.Work.Job.Context.ParentHash == fighter.Job.Context.ParentHash {
				if err := worker.clearScan(); err != nil {
					return false, err
				}
			}
		}
	}
	if ctx.Err() != nil {
		return progress, nil
	}
	if entry.PoolState == "pending" {
		changed, err := worker.deliverPool(ctx, entry)
		if err != nil {
			return progress, err
		}
		progress = progress || changed
	}
	// A pool failure cannot suppress independently qualifying fighter work.
	// Prospect-only work never reaches Submit, whose pool target is independent.
	if ctx.Err() != nil || fighter == nil || !fighter.ProspectWin {
		return progress, nil
	}
	if fighter.Authorization == nil && fighter.AdmissionState == "needs-signature" {
		// D80 never signs a new birth after the original issuance window. Already
		// signed pending work may recover an earlier receipt after that window.
		if worker.now().UnixMilli() >= fighter.Job.ExpiresAtMS {
			fighter.AdmissionState, fighter.LastError = "expired", "expired"
			if err := worker.outbox.update(*entry); err != nil {
				return progress, err
			}
			return true, nil
		}
		call, cancel := context.WithTimeout(ctx, 30*time.Second)
		proof := fighter.Candidate.Proof
		proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
		auth, err := worker.signer(call, fighter.Job.Context, proof)
		cancel()
		if err == nil && (len(auth.PublicKey) != 66 || len(auth.Signature) > 144) {
			err = ErrCorrupt
		}
		if err == nil {
			auth.PublicKey, auth.Signature = strings.ToLower(auth.PublicKey), strings.ToLower(auth.Signature)
		}
		if err != nil || !manifestCanonicalAuth(auth) || gymauth.VerifyAuthorization(fighter.Job.Context, fighter.Candidate.Proof, auth) != nil {
			fighter.LastError = "signer_unavailable"
			if err := worker.outbox.update(*entry); err != nil {
				return progress, err
			}
			return progress, nil
		}
		fighter.Authorization, fighter.AdmissionState, fighter.LastError = &auth, "pending", ""
		if err := worker.outbox.update(*entry); err != nil {
			return progress, err
		}
		progress = true
	}
	if ctx.Err() != nil || fighter.AdmissionState != "pending" {
		return progress, nil
	}
	if !worker.deliveryAuthoritiesMatch() {
		return progress, ErrCorrupt
	}
	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	proof := fighter.Candidate.Proof
	proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	result, err := worker.admission.SubmitManifest(call, fighter.Job, proof, *fighter.Authorization)
	cancel()
	if err == nil {
		if !manifestCanonicalProof(result.Record.Proof) || !manifestCanonicalAuth(result.Record.Authorization) {
			return progress, ErrCorrupt
		}
		result.Record.Proof.MerkleBranch = append([]string{}, result.Record.Proof.MerkleBranch...)
		candidate := *fighter
		candidate.Receipt, candidate.AdmissionState, candidate.LastError = &result, "accepted", ""
		// Local CheckNonce above establishes the real digest. This existing
		// validator derives DNA, receipt binding and eligibility from that digest.
		if validateManifestEntry(candidate, poolManifestConfig(worker.config)) != nil {
			return progress, ErrCorrupt
		}
		*fighter = candidate
		progress = true
	} else {
		fighter.LastError = "outcome_unknown"
		var failure *prospectservice.ClientError
		if errors.As(err, &failure) {
			fighter.LastError = failure.Code
			if !failure.OutcomeUnknown {
				switch failure.Code {
				case "expired":
					fighter.AdmissionState, progress = "expired", true
				case "invalid_request", "too_large":
					fighter.AdmissionState, progress = "rejected", true
				}
			}
		}
	}
	if err := worker.outbox.update(*entry); err != nil {
		return progress, err
	}
	if fighter.AdmissionState == "accepted" && fighter.Receipt != nil {
		if err := worker.notifyPoolReceipt(ctx, fighter.Receipt.Record); err != nil {
			return progress, err
		}
	}
	return progress, nil
}

func (worker *PoolWorker) deliverPool(ctx context.Context, entry *PoolEntry) (bool, error) {
	if !worker.deliveryAuthoritiesMatch() {
		return false, ErrCorrupt
	}
	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	receipt, err := worker.authority.Lookup(call, poolDeliveryWork(entry.Work), entry.Nonce)
	cancel()
	var failure *poolservice.ClientError
	missing := errors.As(err, &failure) && !failure.OutcomeUnknown && failure.Code == "not_found"
	if err == nil {
		if err := worker.retainPoolReceipt(entry, receipt); err != nil {
			return false, err
		}
		if receipt.AcceptedSequence != 0 {
			return true, nil
		}
	}
	// A pending block intent is not earnings. It can be retried with the same
	// job/nonce; an uncertain lookup must never authorize another identity.
	if (missing || err == nil) && ctx.Err() == nil {
		if !worker.deliveryAuthoritiesMatch() {
			return false, ErrCorrupt
		}
		call, cancel = context.WithTimeout(ctx, 30*time.Second)
		receipt, err = worker.authority.Submit(call, poolDeliveryWork(entry.Work), entry.Nonce)
		cancel()
		if err == nil {
			if err := worker.retainPoolReceipt(entry, receipt); err != nil {
				return false, err
			}
			return receipt.AcceptedSequence != 0, nil
		}
	}
	entry.LastError = "outcome_unknown"
	failure = nil
	if errors.As(err, &failure) {
		entry.LastError = failure.Code
		if !failure.OutcomeUnknown && entry.PoolReceipt == nil {
			switch failure.Code {
			case "expired":
				entry.PoolState = "expired"
			case "rejected", "invalid_request", "too_large":
				entry.PoolState = "rejected"
			}
		}
	}
	if err := worker.outbox.update(*entry); err != nil {
		return false, err
	}
	return entry.PoolState != "pending", nil
}

func poolDeliveryWork(work poolshare.MiningWork) poolshare.MiningWork {
	work.Template = cloneManifestScan(ManifestScan{Template: work.Template}).Template
	return work
}

func (worker *PoolWorker) retainPoolReceipt(entry *PoolEntry, receipt poolshare.Submission) error {
	candidate := *entry
	candidate.PoolReceipt, candidate.LastError = &receipt, ""
	if receipt.AcceptedSequence != 0 {
		candidate.PoolState = "accepted"
	}
	if validatePoolEntry(candidate, worker.config) != nil {
		return ErrCorrupt
	}
	if err := worker.outbox.update(candidate); err != nil {
		return err
	}
	*entry = candidate
	return nil
}

func (worker *PoolWorker) notifyPoolReceipt(ctx context.Context, record registry.ManifestRecord) (err error) {
	if worker.observer == nil || ctx.Err() != nil {
		return nil
	}
	defer func() {
		if recover() != nil || err != nil {
			err = ErrManifestObserver
		}
	}()
	raw, err := json.Marshal(record)
	if err != nil || len(raw) > registry.MaxRecordSize {
		return ErrManifestObserver
	}
	record.Proof.MerkleBranch = append([]string{}, record.Proof.MerkleBranch...)
	if worker.observer(ctx, record) != nil {
		return ErrManifestObserver
	}
	return nil
}
