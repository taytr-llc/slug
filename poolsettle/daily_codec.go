// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"time"
)

func dailyDecode(raw []byte, limit int, target any) error {
	if len(raw) == 0 || len(raw) > limit {
		return ErrCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return ErrCorrupt
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(raw, canonical) {
		return ErrCorrupt
	}
	return nil
}
func dailyEncode(value any, limit int) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > limit {
		return nil, ErrCapacity
	}
	return raw, nil
}
func dailyNumber(prefix string, n uint64) []byte {
	k := make([]byte, len(prefix)+8)
	copy(k, prefix)
	binary.BigEndian.PutUint64(k[len(prefix):], n)
	return k
}
func dailySequence(n uint64) []byte       { var b [8]byte; binary.BigEndian.PutUint64(b[:], n); return b[:] }
func dailyReadSequence(raw []byte) uint64 { return binary.BigEndian.Uint64(raw) }
func dailyOutpoint(prefix, txid string, index uint32) []byte {
	k := make([]byte, len(prefix)+len(txid)+4)
	copy(k, prefix)
	copy(k[len(prefix):], txid)
	binary.BigEndian.PutUint32(k[len(k)-4:], index)
	return k
}
func dailyClaimsDigest(ctx context.Context, claims []Claim) (string, error) {
	if ctx == nil || len(claims) > dailySigningClaims {
		return "", ErrInvalid
	}
	h := sha256.New()
	h.Write([]byte("SLUG pool daily change claims\x00v2\x00"))
	w := canonicalSigningWriter{ctx: ctx, h: h}
	w.literal("[")
	for i, c := range claims {
		if i > 0 {
			w.literal(",")
		}
		w.value(c)
	}
	w.literal("]")
	if w.err != nil {
		return "", w.err
	}
	return hex.EncodeToString(h.Sum(nil)), ctx.Err()
}
func dailyBody(ctx context.Context, sequence uint64, b Batch) (dailyBatchBody, error) {
	if sequence == 0 {
		return dailyBatchBody{}, ErrCorrupt
	}
	out := dailyBatchBody{Version: Version2, Sequence: sequence, Day: b.Day, CreatedAtMS: b.CreatedAtMS,
		Inputs: make([]dailyInput, len(b.Inputs)), Payments: append([]Payment(nil), b.Payments...), FeeAtoms: b.FeeAtoms, TxID: b.TxID, Raw: b.Raw}
	for i, c := range b.Inputs {
		out.Inputs[i] = dailyInput{TxID: c.TxID, Index: c.Index}
	}
	if b.Change != nil {
		digest, err := dailyClaimsDigest(ctx, b.Change.Claims)
		if err != nil {
			return dailyBatchBody{}, err
		}
		out.Change = &dailyChange{Index: b.Change.Index, Atoms: b.Change.Atoms, Script: b.Change.Script, ClaimCount: uint64(len(b.Change.Claims)), ClaimsDigest: digest}
	}
	return out, nil
}
func dailyReconstruct(ctx context.Context, r *settlementReplay, body dailyBatchBody, delivery dailyDelivery) (Batch, error) {
	if body.Version != Version2 || body.Sequence != r.seen.Batches+1 || len(body.Inputs) == 0 || len(body.Inputs) > 1464 {
		return Batch{}, ErrCorrupt
	}
	inputs := make([]Coin, 0, len(body.Inputs))
	next := 0
	for _, c := range r.coins {
		if next < len(body.Inputs) && c.TxID == body.Inputs[next].TxID && c.Index == body.Inputs[next].Index {
			inputs = append(inputs, c)
			next++
		}
	}
	if next != len(body.Inputs) {
		return Batch{}, ErrCorrupt
	}
	payments, change, reason, err := planDaily(inputs, r.config, body.FeeAtoms)
	if err != nil || reason != "" || !reflect.DeepEqual(payments, body.Payments) {
		return Batch{}, ErrCorrupt
	}
	if change != nil {
		change.TxID = body.TxID
	}
	b := Batch{Day: body.Day, CreatedAtMS: body.CreatedAtMS, Inputs: inputs, Payments: payments, FeeAtoms: body.FeeAtoms, Change: change, TxID: body.TxID, Raw: body.Raw, State: delivery.State}
	want, err := dailyBody(ctx, body.Sequence, b)
	if err != nil {
		return Batch{}, err
	}
	if !reflect.DeepEqual(want, body) {
		return Batch{}, ErrCorrupt
	}
	return b, nil
}
func dailyValidHeader(h dailyHeader, m dailyMarker) bool {
	if h.Application != dailyApplication || h.Version != Version2 || h.ConfigID != m.ConfigID || h.SourceID != m.SourceID || h.Revision == 0 || h.Revision == math.MaxUint64 || h.Credits > uint64(m.Config.MaxRounds) || h.Batches > uint64(m.Config.MaxBatches) || h.Events != h.Credits+h.Batches || h.LogicalBytes < dailyHeadLimit+dailyReportLimit || h.LogicalBytes > dailyLogicalLimit || h.LastClockMS < 0 || h.LastClockMS > 253402300799999 || h.PendingBatch > h.Batches || h.PendingBatch != 0 && h.PendingBatch != h.Batches {
		return false
	}
	if h.Batches == 0 {
		return h.LastDay == ""
	}
	d, err := time.Parse("2006-01-02", h.LastDay)
	return err == nil && d.Format("2006-01-02") == h.LastDay && h.LastDay <= time.UnixMilli(h.LastClockMS).UTC().Format("2006-01-02")
}
