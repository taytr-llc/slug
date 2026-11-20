// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slugwallet/wallet"
)

type poolWithdrawalResources struct {
	Journal  *walletwithdraw.Journal
	Scope    poolWithdrawalScope
	wallet   *walletRuntime
	seam     poolWithdrawalWallet
	node     *withdrawalNode
	life     context.Context
	cancel   context.CancelFunc
	gate     chan struct{}
	poison   error // protected by gate
	once     sync.Once
	closeErr error
}

func openPoolWithdrawal(ctx context.Context, r poolChildResources, w *walletRuntime) (_ *poolWithdrawalResources, resultErr error) {
	if ctx == nil || ctx.Err() != nil || r.Life == nil || r.Life.Err() != nil || r.Withdrawal == nil || r.Withdrawal.check() != nil || w == nil || !w.mu.TryLock() {
		return nil, walletwithdraw.ErrInvalid
	}
	locked := true
	defer func() {
		if locked {
			w.mu.Unlock()
		}
	}()
	scope := *r.Withdrawal
	pin := scope.Identity.Pin
	source, sourceErr := poolservice.BindingID(r.Source)
	if sourceErr != nil || source != pin.PoolSourceBindingID || scope.guard.Source != r.Source || r.Recipient.Config.Source != r.Source || r.Recipient.Config.Fixture != pin.Fixture {
		return nil, walletwithdraw.ErrInvalid
	}
	if w.closed || w.wallet == nil || w.backend == nil || w.identity != (walletIdentity{pin.RootPublicKey, pin.GymPublicKey, pin.GymID}) || r.Recipient.Config.AccountID != pin.AccountID || r.Recipient.Config.AccountAuthorityID != pin.AccountAuthorityID {
		return nil, walletwithdraw.ErrInvalid
	}
	life, cancel := context.WithCancel(r.Life)
	p := &poolWithdrawalResources{Scope: scope, wallet: w, life: life, cancel: cancel, gate: make(chan struct{}, 1)}
	ok := false
	defer func() {
		if !ok {
			cancel()
			if p.seam != nil {
				resultErr = errors.Join(resultErr, p.seam.Close())
			}
			if p.Journal != nil {
				resultErr = errors.Join(resultErr, p.Journal.Close())
			}
			if p.node != nil {
				p.node.close()
			}
		}
	}()
	var e error
	p.Journal, e = walletwithdraw.Open(life, scope.journalPath, scope.Identity)
	if e != nil {
		return nil, e
	}
	if w.allocation != nil {
		allocator := w.allocation
		w.mu.Unlock()
		locked = false
		p.seam, e = openAllocatedWithdrawalWallet(ctx, allocator, scope.BindingID, false)
	} else {
		p.seam, e = w.wallet.OpenCandidateWithdrawals(ctx, scope.BindingID)
	}
	if e != nil {
		return nil, e
	}
	p.node, e = newWithdrawalNode(r.RPC)
	if e != nil {
		return nil, e
	}
	// Reconcile durable signed exclusions before allowing another selection.
	owner, e := p.Journal.TryAcquire(ctx)
	if e != nil {
		return nil, e
	}
	defer owner.Close()
	walletIDs, e := p.seam.OperationIDs(ctx)
	if e != nil {
		return nil, e
	}
	matched := make(map[string]bool, len(walletIDs))
	var after uint32
	for {
		rows, err := owner.List(after, 16)
		if err != nil {
			return nil, err
		}
		for _, op := range rows {
			record, err := p.seam.Lookup(ctx, op.OperationID)
			if err == nil {
				matched[op.OperationID] = true
			}
			if op.Draft == nil {
				if errors.Is(err, wallet.ErrWithdrawalNotFound) {
					continue
				}
				if err != nil || op.Stage != walletwithdraw.StageIntent || record.State != "prepared" || record.Draft.BindingID != scope.BindingID || record.Draft.Recipient != op.Intent.Destination || record.Draft.AmountAtoms != op.Intent.RecipientAtoms {
					return nil, walletwithdraw.ErrCorrupt
				}
				continue
			}
			if err != nil || record.DraftDigest != op.DraftDigest || record.Draft.BindingID != scope.BindingID {
				return nil, walletwithdraw.ErrCorrupt
			}
			if op.Signed != nil || op.Stage == walletwithdraw.StageAbandoned {
				token, err := owner.Reconciliation(op.OperationID)
				if err != nil {
					return nil, err
				}
				if err = p.reconcile(ctx, token); err != nil {
					return nil, err
				}
			} else if record.State != "prepared" {
				return nil, walletwithdraw.ErrCorrupt
			}
		}
		if len(rows) < 16 {
			break
		}
		after = rows[len(rows)-1].Sequence
	}
	if len(matched) != len(walletIDs) {
		return nil, walletwithdraw.ErrCorrupt
	}
	for _, id := range walletIDs {
		if !matched[id] {
			return nil, walletwithdraw.ErrCorrupt
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	ok = true
	return p, nil
}
func (p *poolWithdrawalResources) begin(ctx context.Context) (context.Context, func(), error) {
	if p == nil || ctx == nil {
		return nil, nil, walletwithdraw.ErrInvalid
	}
	select {
	case p.gate <- struct{}{}:
	default:
		return nil, nil, walletwithdraw.ErrBusy
	}
	if p.life.Err() != nil || p.poison != nil || ctx.Err() != nil {
		<-p.gate
		return nil, nil, walletwithdraw.ErrClosed
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(p.life, cancel)
	return work, func() { stop(); cancel(); <-p.gate }, nil
}
func (p *poolWithdrawalResources) withWallet(ctx context.Context, fn func(*wallet.Wallet) error) error {
	if p.wallet.allocation != nil {
		// The admitted resource owner outlives this call; allocator methods own their
		// wallet transactions and original fences, with no outer wallet mutex held.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fn(p.wallet.wallet)
	}
	w := p.wallet
	if !w.mu.TryLock() {
		return walletwithdraw.ErrBusy
	}
	defer w.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if w.closed || w.wallet == nil || w.backend == nil || w.backend.Disconnected() {
		return walletwithdraw.ErrClosed
	}
	return fn(w.wallet)
}
func (p *poolWithdrawalResources) Prepare(ctx context.Context, r walletwithdraw.Request) (walletwithdraw.Draft, error) {
	work, end, e := p.begin(ctx)
	if e != nil {
		return walletwithdraw.Draft{}, e
	}
	defer end()
	var d wallet.CandidateWithdrawalDraft
	e = p.withWallet(work, func(*wallet.Wallet) error {
		var err error
		d, err = p.seam.Prepare(work, wallet.CandidateWithdrawalRequest{Version: r.Version, OperationID: r.OperationID, Recipient: r.Recipient, AmountAtoms: r.AmountAtoms})
		return err
	})
	if e != nil {
		return walletwithdraw.Draft{}, e
	}
	raw, e := json.Marshal(d)
	if e != nil {
		return walletwithdraw.Draft{}, walletwithdraw.ErrInvalid
	}
	return walletwithdraw.ParseDraft(raw)
}
func (p *poolWithdrawalResources) WalletRecord(ctx context.Context, id string) (wallet.CandidateWithdrawalRecord, error) {
	work, end, e := p.begin(ctx)
	if e != nil {
		return wallet.CandidateWithdrawalRecord{}, e
	}
	defer end()
	var r wallet.CandidateWithdrawalRecord
	e = p.withWallet(work, func(*wallet.Wallet) error { var err error; r, err = p.seam.Lookup(work, id); return err })
	return r, e
}
func (p *poolWithdrawalResources) walletTip(ctx context.Context) (withdrawalTip, error) {
	var t withdrawalTip
	e := p.withWallet(ctx, func(w *wallet.Wallet) error {
		if !w.ChainSynced() {
			return walletwithdraw.ErrConflict
		}
		s := w.SyncedTo()
		if s.Height < 0 {
			return walletwithdraw.ErrConflict
		}
		t = withdrawalTip{s.Hash.String(), uint32(s.Height)}
		return nil
	})
	return t, e
}
func (p *poolWithdrawalResources) inspect(ctx context.Context, d walletwithdraw.Draft) error {
	if d.BindingID != p.Scope.BindingID {
		return walletwithdraw.ErrConflict
	}
	if _, e := walletwithdraw.DraftDigest(d); e != nil {
		return e
	}
	before, e := p.walletTip(ctx)
	if e != nil {
		return e
	}
	if e = p.node.inspect(ctx, d, before); e != nil {
		return e
	}
	after, e := p.walletTip(ctx)
	if e != nil || after != before {
		return walletwithdraw.ErrConflict
	}
	return nil
}
func (p *poolWithdrawalResources) Inspect(ctx context.Context, d walletwithdraw.Draft) error {
	work, end, e := p.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	return p.inspect(work, d)
}
func (p *poolWithdrawalResources) watchChange(ctx context.Context, d walletwithdraw.Draft) error {
	a, e := address.DecodeAddress(d.Change.Address, &chaincfg.MainNetParams)
	if e != nil || a.EncodeAddress() != d.Change.Address {
		return walletwithdraw.ErrInvalid
	}
	// This synchronous inherited call is cancelled by shutting down its exact
	// owned transport; join the callback instead of leaking a waiter goroutine.
	work, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	w := p.wallet
	if !w.mu.TryLock() {
		return walletwithdraw.ErrBusy
	}
	defer w.mu.Unlock()
	if w.closed || w.backend == nil || w.backend.Disconnected() {
		return walletwithdraw.ErrClosed
	}
	done := make(chan struct{})
	stop := context.AfterFunc(work, func() { w.backend.Client.Shutdown(); close(done) })
	e = w.backend.NotifyReceived([]address.Address{a})
	if !stop() {
		<-done
	}
	if e != nil || work.Err() != nil {
		p.poison = walletwithdraw.ErrClosed
		return walletwithdraw.ErrClosed
	}
	return nil
}
func (p *poolWithdrawalResources) Sign(ctx context.Context, d walletwithdraw.Draft) ([]byte, error) {
	work, end, e := p.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer end()
	if e = p.inspect(work, d); e != nil {
		return nil, e
	}
	if p.wallet.allocation == nil {
		if e = p.watchChange(work, d); e != nil {
			return nil, e
		}
	}
	digest, e := walletwithdraw.DraftDigest(d)
	if e != nil {
		return nil, e
	}
	var signed wallet.CandidateWithdrawalSigned
	e = p.withWallet(work, func(w *wallet.Wallet) error {
		if w.Locked() {
			if err := w.Unlock([]byte(fixtureWalletPrivatePass), nil); err != nil {
				return walletwithdraw.ErrUnauthorized
			}
			defer w.Lock()
		}
		var err error
		signed, err = p.seam.Sign(work, d.OperationID, digest)
		return err
	})
	if e != nil {
		return nil, e
	}
	if signed.Version != 1 || signed.OperationID != d.OperationID || signed.DraftDigest != digest || len(signed.RawHex) > 8192 {
		return nil, walletwithdraw.ErrCorrupt
	}
	raw, e := hex.DecodeString(signed.RawHex)
	if e != nil || hex.EncodeToString(raw) != signed.RawHex {
		return nil, walletwithdraw.ErrCorrupt
	}
	verified, e := walletwithdraw.ValidateSigned(d, raw)
	if e != nil || verified.TxID != signed.TxID {
		return nil, walletwithdraw.ErrCorrupt
	}
	if work.Err() != nil {
		return nil, work.Err()
	}
	return raw, nil
}
func (p *poolWithdrawalResources) reconcile(ctx context.Context, r *walletwithdraw.Reconciliation) error {
	if !p.Journal.OwnsReconciliation(r) || r.Check() != nil {
		return walletwithdraw.ErrConflict
	}
	row, e := p.seam.Lookup(ctx, r.OperationID())
	if e != nil || row.Draft.BindingID != p.Scope.BindingID || row.DraftDigest != r.DraftDigest() {
		return walletwithdraw.ErrCorrupt
	}
	switch r.Kind() {
	case "retained":
		e = p.seam.Retain(ctx, r.OperationID(), r.DraftDigest(), r.TxID())
	case "abandoned":
		e = p.seam.Abandon(ctx, r.OperationID(), r.DraftDigest())
	default:
		return walletwithdraw.ErrConflict
	}
	if e != nil {
		return e
	}
	return r.Check()
}
func (p *poolWithdrawalResources) Reconcile(ctx context.Context, r *walletwithdraw.Reconciliation) error {
	work, end, e := p.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	return p.withWallet(work, func(*wallet.Wallet) error { return p.reconcile(work, r) })
}
func (p *poolWithdrawalResources) Deliver(ctx context.Context, d *walletwithdraw.Delivery) (walletwithdraw.DeliveryResult, string, error) {
	work, end, e := p.begin(ctx)
	if e != nil {
		return walletwithdraw.DeliveryUnknown, "node_unavailable", e
	}
	defer end()
	work, cancel := context.WithTimeout(work, 15*time.Second)
	defer cancel()
	if !p.Journal.OwnsDelivery(d) {
		return walletwithdraw.DeliveryUnknown, "node_unavailable", walletwithdraw.ErrUnauthorized
	}
	if _, e = p.walletTip(work); e != nil {
		return walletwithdraw.DeliveryUnknown, "node_unavailable", e
	}
	raw, txid, _, e := d.Take()
	if e != nil {
		return walletwithdraw.DeliveryUnknown, "node_unavailable", e
	}
	result, code := p.node.deliver(work, raw, txid)
	return result, code, nil
}
func (p *poolWithdrawalResources) Observe(ctx context.Context, o *walletwithdraw.Owner, id string) (walletwithdraw.ObservationInput, error) {
	work, end, e := p.begin(ctx)
	if e != nil {
		return walletwithdraw.ObservationInput{}, e
	}
	defer end()
	work, cancel := context.WithTimeout(work, 15*time.Second)
	defer cancel()
	if o == nil {
		return walletwithdraw.ObservationInput{}, walletwithdraw.ErrInvalid
	}
	op, e := o.Lookup(id)
	if e != nil || op.Signed == nil || op.Draft == nil || op.Draft.BindingID != p.Scope.BindingID {
		return walletwithdraw.ObservationInput{}, walletwithdraw.ErrConflict
	}
	r, e := o.Reconciliation(id)
	if e != nil || !p.Journal.OwnsReconciliation(r) {
		return walletwithdraw.ObservationInput{}, walletwithdraw.ErrConflict
	}
	return p.node.observe(work, *op.Signed), nil
}
func (p *poolWithdrawalResources) Close() error {
	if p == nil {
		return nil
	}
	p.once.Do(func() {
		p.cancel()
		p.gate <- struct{}{}
		defer func() { <-p.gate }()
		p.closeErr = errors.Join(p.seam.Close(), p.Journal.Close())
		p.node.close()
	})
	return p.closeErr
}

func (p *poolWithdrawalResources) AbandonUnprepared(ctx context.Context, id string) error {
	work, end, e := p.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	if s, ok := p.seam.(*allocatedWithdrawalWallet); ok {
		return s.abandonUnprepared(work, id)
	}
	return nil
}
