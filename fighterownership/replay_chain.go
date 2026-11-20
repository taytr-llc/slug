// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fightertransfer"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func (r *Replay) startBranch(b *replayBranch) error {
	b.effective = map[string]EffectiveState{}
	b.winners = map[string]string{}
	b.commits = map[string]TransactionWitness{}
	b.checkpoints = map[string]PublicationEntry{}
	b.conflicted = map[string]EffectiveState{}
	for n := uint32(1); n <= b.target.AdmissionSequence; n++ {
		a, ok := r.store.admissions[n]
		if !ok || !r.verifiedAdmissions[n] {
			return ErrUnavailable
		}
		v, e := InitialState(r.store.pin, a)
		if e != nil {
			return e
		}
		b.effective[v.ProspectID] = v
		r.states[v.StateID] = v
	}
	for prospect, v := range r.official {
		if _, ok := b.effective[prospect]; !ok {
			return ErrUnavailable
		}
		b.effective[prospect] = v
		var ev FinalStateEvidence
		if json.Unmarshal(v.Evidence, &ev) != nil {
			return ErrCorrupt
		}
		if ev.SixtiethDescendantHeight > b.target.ChainHeight || b.hashes[ev.SixtiethDescendantHeight] != ev.SixtiethDescendantHash {
			b.conflicted[prospect] = v
		}
	}
	for n := uint32(1); n <= b.target.PublicationSequence; n++ {
		p, ok := r.store.publications[n]
		if !ok {
			return ErrUnavailable
		}
		if p.Kind != "checkpoint" {
			continue
		}
		var ref struct {
			Checkpoint chaincfg.SignedCheckpoint `json:"checkpoint"`
		}
		if json.Unmarshal(p.Payload, &ref) != nil {
			return ErrCorrupt
		}
		key := ref.Checkpoint.Hash.String()
		if _, exists := b.checkpoints[key]; !exists {
			b.checkpoints[key] = p
		}
	}
	return nil
}
func cloneBranch(b *replayBranch) *replayBranch {
	x := *b
	x.effective = map[string]EffectiveState{}
	for k, v := range b.effective {
		x.effective[k] = v
	}
	x.winners = map[string]string{}
	for k, v := range b.winners {
		x.winners[k] = v
	}
	x.commits = map[string]TransactionWitness{}
	for k, v := range b.commits {
		x.commits[k] = v
	}
	return &x
}
func boundedTx(ctx context.Context, raw []byte, cap int) (*wire.MsgTx, error) {
	if len(raw) > cap {
		return nil, ErrCapacity
	}
	r := blockReader{raw: raw}
	_, _, e := walkTx(ctx, &r)
	if e != nil || r.pos != len(raw) {
		return nil, ErrCorrupt
	}
	var tx wire.MsgTx
	if tx.Deserialize(bytes.NewReader(raw)) != nil {
		return nil, ErrCorrupt
	}
	return &tx, nil
}
func selectedParentOutputs(ctx context.Context, raw []byte, wanted map[uint32]bool) (map[uint32]*wire.TxOut, error) {
	r := blockReader{raw: raw}
	_, _, e := walkTx(ctx, &r)
	if e != nil || r.pos != len(raw) {
		return nil, ErrCorrupt
	}
	r.pos = 4
	r.err = nil
	if raw[r.pos] == 0 {
		r.take(2)
	}
	n := r.count()
	for i := uint64(0); i < n; i++ {
		r.take(36)
		r.blob()
		r.take(4)
	}
	n = r.count()
	out := map[uint32]*wire.TxOut{}
	for i := uint64(0); i < n; i++ {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		atoms := int64(r.num(8))
		script := r.take(r.count())
		if wanted[uint32(i)] {
			if len(script) != 22 {
				return nil, ErrInvalid
			}
			out[uint32(i)] = &wire.TxOut{Value: atoms, PkScript: append([]byte(nil), script...)}
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	return out, nil
}
func (r *Replay) carrier(ctx context.Context, b *replayBranch, h HeaderRow, txs []parsedBlockTx, index int, newStates map[string]EffectiveState) (*CarrierWitness, error) {
	item := txs[index]
	d := item.discovery
	if d == nil || (!d.RevealLocator && !d.EmbeddedEnvelope) {
		return nil, nil
	}
	if len(item.raw) > 12000 {
		return nil, ErrCapacity
	}
	spend, e := boundedTx(ctx, item.raw, 12000)
	if e != nil {
		return nil, e
	}
	if len(spend.TxIn) != 1 {
		return nil, nil
	}
	token := txscript.MakeScriptTokenizer(0, spend.TxIn[0].SignatureScript)
	var redeem []byte
	for token.Next() {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		redeem = token.Data()
	}
	if token.Err() != nil {
		return nil, nil
	}
	env, _, e := fightertransfer.ParseRedeem(redeem)
	if e != nil {
		return nil, nil
	}
	if hex.EncodeToString(env.Intent.PinID[:]) != r.store.pinID {
		return nil, nil
	}
	priorID := hex.EncodeToString(env.Intent.PriorStateID[:])
	prior, known := newStates[priorID]
	if !known {
		prior, known = r.states[priorID]
	}
	if !known {
		return nil, nil
	} // Complete earlier chain and signed prefixes close the reachable state set.
	current, ok := b.effective[prior.ProspectID]
	if !ok || current.StateID != priorID {
		return nil, nil
	}
	commit, ok := b.commits[spend.TxIn[0].PreviousOutPoint.Hash.String()]
	if !ok {
		return nil, nil
	} // The complete preceding scan found no matching carrier commit.
	if commit.Height > h.Height || commit.Height == h.Height && commit.TransactionIndex >= uint32(index) {
		return nil, nil
	}
	if prior.Generation > 0 {
		var ev FinalStateEvidence
		json.Unmarshal(prior.Evidence, &ev)
		if ev.SixtiethDescendantHeight >= commit.Height {
			return nil, nil
		}
	}
	commitRaw, _ := hex.DecodeString(commit.Raw)
	c, e := boundedTx(ctx, commitRaw, 4096)
	if e != nil {
		return nil, e
	}
	if len(c.TxIn) < 1 || len(c.TxIn) > 16 {
		return nil, nil
	}
	parents := make([][]byte, 0, len(c.TxIn))
	rawByID := map[string][]byte{}
	total := len(commitRaw) + len(item.raw)
	wanted := map[string]map[uint32]bool{}
	for _, in := range c.TxIn {
		id := in.PreviousOutPoint.Hash.String()
		if wanted[id] == nil {
			wanted[id] = map[uint32]bool{}
		}
		wanted[id][in.PreviousOutPoint.Index] = true
		if _, ok := rawByID[id]; ok {
			continue
		}
		raw, e := r.node.Transaction(ctx, id)
		if e != nil {
			return nil, errors.Join(ErrUnavailable, e)
		}
		if len(raw) > fightertransfer.MaxEvidenceSize-total {
			return nil, ErrCapacity
		}
		total += len(raw)
		rawByID[id] = raw
		parents = append(parents, raw)
	}
	var expected fightertransfer.Expected
	copy(expected.PinID[:], env.Intent.PinID[:])
	copy(expected.PriorStateID[:], env.Intent.PriorStateID[:])
	key, _ := hex.DecodeString(prior.OwnerPublicKey)
	copy(expected.SenderGym[:], key)
	report, e := fightertransfer.ValidateSupplied(ctx, expected, fightertransfer.Evidence{CommitRaw: commitRaw, SpendRaw: item.raw, FundingTransactions: parents})
	if e != nil {
		if errors.Is(e, fightertransfer.ErrInvalid) {
			return nil, nil
		}
		return nil, e
	}
	prevouts := make([]FundingPrevout, 0, len(report.Funding))
	outputs := map[string]map[uint32]*wire.TxOut{}
	for id, raw := range rawByID {
		x, e := selectedParentOutputs(ctx, raw, wanted[id])
		if e != nil {
			return nil, e
		}
		outputs[id] = x
	}
	for _, op := range report.Funding {
		out := outputs[op.Hash.String()][op.Index]
		if out == nil {
			return nil, ErrUnavailable
		}
		prevouts = append(prevouts, FundingPrevout{op.Hash.String(), op.Index, out.Value, hex.EncodeToString(out.PkScript)})
	}
	envelope, _ := fightertransfer.MarshalEnvelope(report.Envelope)
	tid := hex.EncodeToString(report.TransferID[:])
	w := CarrierWitness{1, r.store.pinID, tid, priorID, commit, transactionWitness(h, txs, index), prevouts, hex.EncodeToString(envelope), string(report.Kind)}
	if e = validateWitness(r.store.pin, w); e != nil {
		return nil, e
	}
	if report.Kind == fightertransfer.Refund {
		return &w, nil
	}
	if old, exists := b.winners[priorID]; exists {
		if old != tid {
			return &w, nil
		}
	} else {
		b.winners[priorID] = tid
	}
	if len(b.conflicted) > 0 {
		return &w, nil
	}
	cp, ok := b.checkpoints[h.Hash]
	if !ok || h.Height > b.target.ChainHeight || b.target.ChainHeight-h.Height < 60 {
		return &w, nil
	}
	var checkpoint struct {
		Checkpoint chaincfg.SignedCheckpoint `json:"checkpoint"`
	}
	json.Unmarshal(cp.Payload, &checkpoint)
	if checkpoint.Checkpoint.Height != int32(h.Height) {
		return nil, ErrCorrupt
	}
	cpid, _ := registry.PublicPublicationID(r.store.pin, cp)
	ev := FinalStateEvidence{1, r.store.pinID, prior.ProspectID, priorID, tid, cpid, h.Height + 60, b.hashes[h.Height+60], hex.EncodeToString(env.Intent.RecipientGym[:])}
	stateID, e := objectDigest("SLUG fighter ownership state", ev)
	if e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(ev)
	state := EffectiveState{1, r.store.pinID, prior.ProspectID, stateID, ev.OwnerPublicKey, prior.BirthAdmissionID, prior.Generation + 1, raw}
	if ValidateEffectiveState(r.store.pin, state) != nil {
		return nil, ErrCorrupt
	}
	b.effective[state.ProspectID] = state
	newStates[stateID] = state
	return &w, nil
}
func (r *Replay) scanBlock(ctx context.Context, b *replayBranch) error {
	h := r.store.headers[b.hashes[b.next]]
	raw, e := r.node.Block(ctx, h.Hash)
	if e != nil {
		return e
	}
	txs, e := parseBlock(ctx, raw, h)
	if e != nil {
		return e
	}
	next := cloneBranch(b)
	newStates := map[string]EffectiveState{}
	witnesses := map[string]CarrierWitness{}
	ordered := make([]string, 0)
	batch := new(leveldb.Batch)
	for index, item := range txs {
		if e = ctx.Err(); e != nil {
			return e
		}
		d := item.discovery
		if d == nil {
			continue
		}
		next.recognized++
		if next.recognized > 8192 {
			return ErrCapacity
		}
		if d.CommitLocator {
			if len(item.raw) > 4096 {
				return ErrCapacity
			}
			if len(next.commits) >= 4096 {
				return ErrCapacity
			}
			next.commits[item.id.String()] = transactionWitness(h, txs, index)
		}
		w, e := r.carrier(ctx, next, h, txs, index, newStates)
		if e != nil {
			return e
		}
		if w == nil {
			continue
		}
		id, e := objectDigest("SLUG fighter ownership witness", *w)
		if e != nil {
			return e
		}
		witnesses[id] = *w
		ordered = append(ordered, id)
		encoded, _ := json.Marshal(w)
		if len(encoded) > 65536 {
			return ErrCapacity
		}
		batch.Put(idKey("w/", id), encoded)
		batch.Put(append(idKey("t/", w.TransferID), idKey("", id)...), []byte{})
	}
	for id, state := range newStates {
		raw, _ := json.Marshal(state)
		batch.Put(idKey("s/", id), raw)
	}
	work := r.work
	work.NextHeight = b.next + 1
	if b.frame != nil {
		work.NextHeight = 0
	} // Historical frame audit cannot advance the live target cursor.
	work.ScanDigest, e = objectDigest("SLUG fighter ownership scan block", struct {
		PreviousDigest   string   `json:"previous_digest"`
		Height           uint32   `json:"height"`
		Hash             string   `json:"hash"`
		TransactionCount uint32   `json:"transaction_count"`
		WitnessIDs       []string `json:"witness_ids"`
	}{work.ScanDigest, h.Height, h.Hash, uint32(len(txs)), ordered})
	if e != nil {
		return e
	}
	next.next++
	return r.save(ctx, batch, work, func() {
		*b = *next
		for id, state := range newStates {
			r.states[id] = state
			r.store.states[id] = state
		}
		for id, w := range witnesses {
			r.newWitnesses[id] = w
			r.store.witnesses[id] = w
		}
	})
}
func (r *Replay) branchMap(b *replayBranch) (OwnershipMap, error) {
	mp := OwnershipMap{1, r.store.pinID, make([]MapItem, 0, b.target.AdmissionSequence)}
	for n := uint32(1); n <= b.target.AdmissionSequence; n++ {
		a, ok := r.store.admissions[n]
		if !ok {
			return mp, ErrUnavailable
		}
		state, ok := b.effective[a.Record.Verified.ProspectID]
		if !ok {
			return mp, ErrUnavailable
		}
		mp.Items = append(mp.Items, MapItem{n, state.ProspectID, state.StateID})
	}
	return mp, ValidateOwnershipMap(r.store.pin, mp)
}
func (r *Replay) completeBranch(ctx context.Context, b *replayBranch) error {
	mp, e := r.branchMap(b)
	if e != nil {
		return e
	}
	id, e := OwnershipMapID(r.store.pin, mp)
	if e != nil {
		return e
	}
	fp, _ := FinalizedPrefixID(r.store.pin, mp)
	if b.frame != nil {
		if id != b.frame.OwnershipMapID || fp != b.frame.FinalizedPrefixID {
			return ErrCorrupt
		}
		fid, _ := FinalityFrameID(r.store.pin, *b.frame)
		for prospect, state := range b.effective {
			if state.Generation > 0 {
				r.official[prospect] = state
				if _, exists := r.officialFrontier[state.StateID]; !exists {
					r.officialFrontier[state.StateID] = FrontierEvidence{"finality_frame", fid}
				}
			}
		}
	}
	batch := new(leveldb.Batch)
	raw, _ := json.Marshal(mp)
	batch.Put(idKey("m/", id), raw)
	work := r.work
	work.WorkingMapID = id
	meta, witnesses, e := r.retainFrame(b, batch)
	if e != nil {
		return e
	}
	return r.saveMeta(ctx, batch, work, meta, func() {
		r.store.maps[id] = mp
		if meta != nil {
			r.store.frames[b.frame.Sequence] = *b.frame
			r.store.frameBodies[b.frame.Sequence] = b.frame.FinalityFrameBody
			for key, w := range witnesses {
				r.store.finalities[key] = w
			}
		}
	})
}
func sortedIDs(v ...string) []string {
	sort.Strings(v)
	out := make([]string, 0, len(v))
	for _, x := range v {
		if len(out) == 0 || out[len(out)-1] != x {
			out = append(out, x)
		}
	}
	return out
}
