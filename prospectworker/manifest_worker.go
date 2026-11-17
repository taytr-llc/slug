// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectworker

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

// ManifestWorker is a single-run, single-hash lifecycle. Cancel Run's context to stop;
// it drains the current Argon attempt and synchronous journal writes first.
// Callers may read Status concurrently and close dependencies after Run returns.
type ManifestWorker struct {
	config    ManifestConfig
	authority ManifestAuthority
	node      Node
	signer    Signer
	outbox    *ManifestOutbox
	observer  ManifestReceiptObserver
	mu        sync.Mutex
	status    Status
	started   bool
	// Private deterministic seams; production callers cannot select test hashes.
	hash         func(*wire.BlockHeader) chainhash.Hash
	now          func() time.Time
	wait         func(context.Context, time.Duration) error
	lastTime     int64
	current      *ManifestScan
	recentHeader string
	recentHash   chainhash.Hash
}

func NewManifest(config ManifestConfig, authority ManifestAuthority, node Node, signer Signer, outbox *ManifestOutbox) (*ManifestWorker, error) {
	return NewManifestWithObserver(config, authority, node, signer, outbox, nil)
}

func NewManifestWithObserver(config ManifestConfig, authority ManifestAuthority, node Node, signer Signer, outbox *ManifestOutbox, observer ManifestReceiptObserver) (*ManifestWorker, error) {
	if err := manifestProduction(); err != nil {
		return nil, err
	}
	canonical, err := ValidateManifestConfig(config)
	if err != nil {
		return nil, err
	}
	if authority == nil || node == nil || signer == nil || outbox == nil || authority.ProtocolVersion() != 3 || authority.AuthorityOrigin() != canonical.AuthorityOrigin || authority.AuthorityPin() != canonical.AuthorityPin || authority.TransportTrustID() != canonical.TransportTrustID {
		return nil, errors.New("explicit D69 authority, node, signer and outbox are required")
	}
	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if outbox.state() != nil || !sameManifestIdentity(outbox.config, canonical) || outbox.config.MaxBytes != canonical.MaxBytes || outbox.config.MaxRecords != canonical.MaxRecords {
		return nil, errors.New("worker configuration does not match outbox")
	}
	return &ManifestWorker{config: canonical, authority: authority, node: node, signer: signer, outbox: outbox, observer: observer, status: Status{State: "ready"}, hash: func(header *wire.BlockHeader) chainhash.Hash { return header.BlockHash() }, now: time.Now, wait: waitFor}, nil
}
func (worker *ManifestWorker) Status() Status {
	worker.mu.Lock()
	defer worker.mu.Unlock()
	return worker.status
}
func (worker *ManifestWorker) set(state, code string) {
	pending := worker.outbox.pendingCount()
	worker.outbox.mu.Lock()
	records, size := worker.outbox.meta.Records, worker.outbox.meta.Bytes
	worker.outbox.mu.Unlock()
	worker.mu.Lock()
	worker.status.State = state
	worker.status.LastError = code
	worker.status.Records = records
	worker.status.Bytes = size
	worker.status.Pending = pending
	worker.mu.Unlock()
}
func (worker *ManifestWorker) Run(ctx context.Context) error {
	if err := manifestProduction(); err != nil {
		return err
	}
	worker.mu.Lock()
	if worker.started {
		worker.mu.Unlock()
		return errors.New("worker cannot be restarted")
	}
	worker.started = true
	worker.mu.Unlock()
	startedAt := time.Now()
	worker.outbox.mu.Lock()
	if err := worker.outbox.state(); err != nil {
		worker.outbox.mu.Unlock()
		return err
	}
	if worker.outbox.running {
		worker.outbox.mu.Unlock()
		return ErrInUse
	}
	worker.outbox.running = true
	worker.outbox.mu.Unlock()
	defer func() {
		worker.outbox.mu.Lock()
		worker.outbox.running = false
		worker.outbox.mu.Unlock()
		worker.set("stopped", worker.Status().LastError)
	}()
	backoff := time.Second
	var refreshed time.Time
	recovered, exists, err := worker.outbox.scan()
	if err != nil {
		return err
	}
	if exists {
		worker.current = &recovered
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		pending, found, err := worker.outbox.nextDue(worker.now().UnixMilli())
		if err != nil {
			return err
		}
		// One due recovery operation per scan cycle prevents a permanently stale
		// block from starving fresh work. Its public evidence is never discarded.
		if found {
			if err := worker.reconcile(ctx, &pending); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return nil
			}
		}
		now := worker.now()
		if now.UnixMilli() < worker.lastTime {
			worker.set("paused", "clock")
			if worker.wait(ctx, time.Second) != nil {
				return nil
			}
			continue
		}
		worker.lastTime = now.UnixMilli()
		if worker.current != nil && now.UnixMilli() >= worker.current.Job.ExpiresAtMS-renewalLead.Milliseconds() {
			if err := worker.clearScan(); err != nil {
				return err
			}
		}
		if worker.current == nil {
			worker.outbox.mu.Lock()
			full := worker.outbox.meta.Records >= worker.config.MaxRecords
			worker.outbox.mu.Unlock()
			if full {
				worker.set("paused", "capacity")
				if worker.wait(ctx, time.Second) != nil {
					return nil
				}
				continue
			}
			worker.set("preparing", "")
			fresh, err := worker.prepare(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				worker.set("retrying", "unavailable")
				if worker.wait(ctx, backoff) != nil {
					return nil
				}
				backoff = min(30*time.Second, backoff*2)
				continue
			}
			if err := worker.outbox.saveScan(&fresh); err != nil {
				if errors.Is(err, ErrCapacity) {
					worker.set("paused", "capacity")
					if worker.wait(ctx, time.Second) != nil {
						return nil
					}
					continue
				}
				return err
			}
			owned := cloneManifestScan(fresh)
			worker.current = &owned
			refreshed = worker.now()
			backoff = time.Second
		}
		scan := worker.current
		if worker.now().UnixMilli() < scan.Job.IssuedAtMS {
			worker.set("paused", "clock")
			if worker.wait(ctx, time.Second) != nil {
				return nil
			}
			continue
		}
		if err := worker.outbox.reserve(*scan); err != nil {
			if errors.Is(err, ErrCapacity) {
				worker.set("paused", "capacity")
				if worker.wait(ctx, time.Second) != nil {
					return nil
				}
				continue
			}
			return err
		}
		if refreshed.IsZero() || worker.now().Sub(refreshed) >= refreshInterval {
			call, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := worker.node.RecheckContext(call, scan.Job.Context)
			cancel()
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				if err := worker.clearScan(); err != nil {
					return err
				}
				worker.set("retrying", "stale")
				if worker.wait(ctx, backoff) != nil {
					return nil
				}
				backoff = min(30*time.Second, backoff*2)
				continue
			}
			refreshed = worker.now()
		}
		if scan.NextNonce == 1<<32 {
			if scan.ExtraNonce == math.MaxUint64 {
				if err := worker.clearScan(); err != nil {
					return err
				}
				continue
			}
			scan.ExtraNonce++
			scan.NextNonce = 0
			candidate, err := prospectwork.BuildManifest(scan.Job, scan.Template, worker.config.PayoutAddress, scan.ExtraNonce)
			if err != nil {
				return err
			}
			scan.Candidate = candidate
			if err := worker.outbox.saveScan(scan); err != nil {
				return err
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		if worker.now().UnixMilli() >= scan.Job.ExpiresAtMS-renewalLead.Milliseconds() {
			continue
		}
		worker.mu.Lock()
		worker.status.JobID = scan.Job.ID
		worker.status.ParentHash = scan.Job.Context.ParentHash
		worker.mu.Unlock()
		worker.set("hashing", "")
		// Full candidate construction/validation happened once for this immutable
		// scan. A nonce attempt needs only the exact 80-byte header.
		headerRaw, _ := hex.DecodeString(scan.Candidate.Proof.Header)
		var header wire.BlockHeader
		if header.Deserialize(bytes.NewReader(headerRaw)) != nil {
			return ErrCorrupt
		}
		header.Nonce = uint32(scan.NextNonce)
		digest := worker.hash(&header)
		worker.mu.Lock()
		worker.status.Attempts++
		worker.status.HashesPerSecond = float64(worker.status.Attempts) / time.Since(startedAt).Seconds()
		worker.mu.Unlock()
		nonce := uint32(scan.NextNonce)
		scan.NextNonce++
		blockWin, prospectWin := classifyManifest(digest, scan.Job)
		if blockWin || prospectWin {
			candidate := withNonce(scan.Candidate, nonce)
			worker.recentHeader = candidate.Proof.Header
			worker.recentHash = digest
			entry := ManifestEntry{Version: Version2, WorkHash: digest.String(), Job: scan.Job, Candidate: candidate, BlockWin: blockWin, ProspectWin: prospectWin, BlockState: "not-applicable", AdmissionState: "not-applicable"}
			if blockWin {
				entry.BlockState = "pending"
			}
			if prospectWin {
				entry.AdmissionState = "needs-signature"
			}
			saved, err := worker.outbox.found(entry, *scan)
			if err != nil {
				return err
			}
			// A newly found block bypasses old recovery deadlines. In particular no
			// older pending signer may delay its first broadcast after durable Sync.
			if blockWin && ctx.Err() == nil {
				if err := worker.reconcile(ctx, &saved); err != nil {
					return err
				}
			}
		} else if err := worker.outbox.advance(*scan); err != nil {
			return err
		}
		// Cancellation during hashing still persists found work before returning.
	}
}
func (worker *ManifestWorker) reconcile(ctx context.Context, pending *ManifestEntry) error {
	worker.set("delivering", pending.LastError)
	if _, err := worker.deliver(ctx, pending); err != nil {
		return err
	}
	if !manifestDone(*pending) {
		if pending.RetryCount < 32 {
			pending.RetryCount++
		}
		delay := retryDelay(pending.RetryCount)
		now := worker.now().UnixMilli()
		if now < 0 || now > math.MaxInt64-delay.Milliseconds() {
			return errors.New("invalid worker clock")
		}
		pending.NextAttemptMS = now + delay.Milliseconds()
	} else {
		pending.NextAttemptMS = 0
	}
	if err := worker.outbox.update(*pending); err != nil {
		return err
	}
	worker.mu.Lock()
	worker.status.RecoveryError = pending.LastError
	worker.status.NextRecoveryMS = pending.NextAttemptMS
	worker.mu.Unlock()
	return nil
}
func (worker *ManifestWorker) clearScan() error {
	if err := worker.outbox.saveScan(nil); err != nil {
		return err
	}
	worker.current = nil
	return nil
}

func (worker *ManifestWorker) prepare(ctx context.Context) (ManifestScan, error) {
	if !worker.authorityMatches() {
		return ManifestScan{}, ErrCorrupt
	}
	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := call.Err(); err != nil {
		return ManifestScan{}, err
	}
	job, err := worker.authority.IssueManifestJob(call, worker.config.PublicKey)
	if err != nil {
		return ManifestScan{}, err
	}
	if err := call.Err(); err != nil {
		return ManifestScan{}, err
	}
	now := worker.now().UnixMilli()
	if prospectjob.ValidateManifest(job) != nil || !matchesManifestPolicy(job, worker.config) || job.PublicKey != worker.config.PublicKey || job.Context.Version != 1 || job.Context.Network != worker.config.Policy.Network || job.Context.Genesis != worker.config.Policy.Genesis || job.Context.DNAVersion != worker.config.Policy.DNAVersion || job.Context.ProspectTarget != worker.config.Policy.ProspectTarget || now < job.IssuedAtMS || now >= job.ExpiresAtMS-renewalLead.Milliseconds() {
		return ManifestScan{}, errors.New("job has no current mining interval")
	}
	template, err := worker.node.ResolveTemplate(call, worker.config.Policy)
	if err != nil {
		return ManifestScan{}, err
	}
	if err := call.Err(); err != nil {
		return ManifestScan{}, err
	}
	if !manifestTemplateBounds(template) {
		return ManifestScan{}, ErrCorrupt
	}
	owned := cloneManifestScan(ManifestScan{Template: template})
	template = owned.Template
	candidate, err := prospectwork.BuildManifest(job, template, worker.config.PayoutAddress, 0)
	if err != nil {
		return ManifestScan{}, err
	}
	if err := call.Err(); err != nil {
		return ManifestScan{}, err
	}
	if err := worker.node.Proposal(call, job.Context, candidate.BlockHex); err != nil {
		return ManifestScan{}, err
	}
	now = worker.now().UnixMilli()
	if now < job.IssuedAtMS || now >= job.ExpiresAtMS-renewalLead.Milliseconds() || call.Err() != nil {
		return ManifestScan{}, errors.New("job preparation expired")
	}
	return ManifestScan{Version: Version2, Job: job, Template: template, Candidate: candidate}, nil
}
func (worker *ManifestWorker) deliver(ctx context.Context, entry *ManifestEntry) (bool, error) {
	// Recompute the actual header digest before trusting any disk classification
	// or sending work. Hash memoization can reuse only exact locally hashed bytes.
	block, err := manifestWorkBlock(entry.Job, entry.Candidate, worker.config)
	if err != nil {
		return false, err
	}
	digest := worker.recentHash
	if worker.recentHeader != entry.Candidate.Proof.Header {
		digest = worker.hash(&block.Header)
		worker.recentHeader = entry.Candidate.Proof.Header
		worker.recentHash = digest
	}
	if digest.String() != entry.WorkHash {
		return false, ErrCorrupt
	}
	if ctx.Err() != nil {
		return false, nil
	}
	progress := false
	if entry.BlockWin && entry.BlockState != "canonical" && entry.BlockState != "rejected" {
		call, cancel := context.WithTimeout(ctx, 30*time.Second)
		// D69: public found work is already durable. Broadcast before any signer
		// or admission call, including when the block misses the prospect target.
		err := worker.node.SubmitBlock(call, entry.Job.Context, entry.Candidate.BlockHex)
		cancel()
		entry.BlockState = "submitted"
		entry.LastError = ""
		if err != nil {
			var submission *prospectnode.SubmissionError
			if errors.As(err, &submission) {
				entry.LastError = submission.Code
			} else {
				entry.LastError = "outcome_unknown"
			}
		}
		if err := worker.outbox.update(*entry); err != nil {
			return false, err
		}
		if ctx.Err() != nil {
			return false, nil
		}
		call, cancel = context.WithTimeout(ctx, 30*time.Second)
		guardProof := entry.Candidate.Proof
		guardProof.MerkleBranch = append([]string{}, guardProof.MerkleBranch...)
		err = worker.node.CanonicalBlock(call, entry.Job.Context, guardProof, entry.WorkHash)
		cancel()
		if err == nil {
			entry.BlockState = "canonical"
			if worker.current != nil && worker.current.Job.Context.ParentHash == entry.Job.Context.ParentHash {
				if err := worker.clearScan(); err != nil {
					return false, err
				}
			}
			entry.LastError = ""
			progress = true
			if err := worker.outbox.update(*entry); err != nil {
				return false, err
			}
		}
	}
	if ctx.Err() != nil {
		return progress, nil
	}
	if entry.ProspectWin && entry.Authorization == nil {
		call, cancel := context.WithTimeout(ctx, 30*time.Second)
		proof := entry.Candidate.Proof
		proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
		auth, err := worker.signer(call, entry.Job.Context, proof)
		cancel()
		if err == nil && (len(auth.PublicKey) != 66 || len(auth.Signature) > 144) {
			err = ErrCorrupt
		}
		if err == nil {
			auth.PublicKey = strings.ToLower(auth.PublicKey)
			auth.Signature = strings.ToLower(auth.Signature)
		}
		if err != nil || !manifestCanonicalAuth(auth) || gymauth.VerifyAuthorization(entry.Job.Context, entry.Candidate.Proof, auth) != nil {
			entry.LastError = "signer_unavailable"
			if err := worker.outbox.update(*entry); err != nil {
				return false, err
			}
			return false, nil
		}
		entry.Authorization = &auth
		entry.AdmissionState = "pending"
		entry.LastError = ""
		if err := worker.outbox.update(*entry); err != nil {
			return false, err
		}
		progress = true
	}
	if ctx.Err() != nil {
		return progress, nil
	}
	if entry.ProspectWin && entry.AdmissionState == "pending" {
		if !worker.authorityMatches() {
			return false, ErrCorrupt
		}
		call, cancel := context.WithTimeout(ctx, 30*time.Second)
		submitProof := entry.Candidate.Proof
		submitProof.MerkleBranch = append([]string{}, submitProof.MerkleBranch...)
		result, err := worker.authority.SubmitManifest(call, entry.Job, submitProof, *entry.Authorization)
		cancel()
		if err == nil {
			if !manifestCanonicalProof(result.Record.Proof) || !manifestCanonicalAuth(result.Record.Authorization) {
				return false, ErrCorrupt
			}
			result.Record.Proof.MerkleBranch = append([]string{}, result.Record.Proof.MerkleBranch...)
			verified, verifyErr := prospect.Verify(entry.Job.Context, entry.Candidate.Proof)
			if verifyErr != nil || !reflect.DeepEqual(verified, result.Record.Verified) {
				return false, ErrCorrupt
			}
			candidateEntry := *entry
			candidateEntry.Receipt = &result
			candidateEntry.AdmissionState = "accepted"
			if validateManifestEntry(candidateEntry, worker.config) != nil {
				return false, ErrCorrupt
			}
			entry.Receipt = &result
			entry.AdmissionState = "accepted"
			entry.LastError = ""
			progress = true
		} else {
			entry.LastError = "outcome_unknown"
			var failure *prospectservice.ClientError
			if errors.As(err, &failure) {
				entry.LastError = failure.Code
				if !failure.OutcomeUnknown {
					switch failure.Code {
					case "expired":
						entry.AdmissionState = "expired"
						progress = true
					case "invalid_request", "too_large":
						entry.AdmissionState = "rejected"
						progress = true
					}
				}
			}
		}
		if err := worker.outbox.update(*entry); err != nil {
			return false, err
		}
		if entry.AdmissionState == "accepted" && entry.Receipt != nil {
			if err := worker.notifyReceipt(ctx, entry.Receipt.Record); err != nil {
				return progress, err
			}
		}
	}
	return progress, nil
}

func (worker *ManifestWorker) authorityMatches() bool {
	a, c := worker.authority, worker.config
	return a.ProtocolVersion() == 3 && a.AuthorityOrigin() == c.AuthorityOrigin && a.AuthorityPin() == c.AuthorityPin && a.TransportTrustID() == c.TransportTrustID
}
