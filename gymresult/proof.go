// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
	"github.com/taytr-llc/slug/wire/v2"
	stdhash "hash"
	"io"
	"sort"
)

type proofWriter struct {
	dst io.Writer
	h   stdhash.Hash
	n   uint32
}

func (w *proofWriter) Write(b []byte) (int, error) {
	if uint64(w.n)+uint64(len(b)) > MaxProofSize {
		return 0, ErrCapacity
	}
	n, e := w.dst.Write(b)
	if n > 0 {
		w.h.Write(b[:n])
		w.n += uint32(n)
	}
	if e == nil && n != len(b) {
		e = io.ErrShortWrite
	}
	return n, e
}
func proofSourceSame(a, b gymbout.Status) bool {
	a.ObservedAtMS = 0
	b.ObservedAtMS = 0
	a.Source.ObservedAtMS = 0
	b.Source.ObservedAtMS = 0
	return equal(a, b)
}
func (s *Store) ExportProof(ctx context.Context, bouts *gymbout.Store, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, saturday, boutID string, p *seednode.ResultPublisher, unpublished io.Writer, prepare func(ProofSummary) (func(func() error) error, error)) (ProofSummary, error) {
	if p == nil || unpublished == nil || prepare == nil {
		return ProofSummary{}, ErrInvalid
	}
	c, done, e := s.call(sourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return ProofSummary{}, e
	}
	defer done()
	if e = s.publisher(p); e != nil {
		return ProofSummary{}, e
	}
	return s.export(c, p, unpublished, prepare)
}
func (s *Store) export(c sourceCall, p *seednode.ResultPublisher, dst io.Writer, prepare func(ProofSummary) (func(func() error) error, error)) (ProofSummary, error) {
	r, st, e := s.read(c, gymaccount.View, true)
	if e != nil {
		return ProofSummary{}, e
	}
	if r.Publication == nil || r.Inclusion == nil {
		return ProofSummary{}, ErrProofUnavailable
	}
	bout, e := c.bouts.Read(c.ctx, c.cards, c.seeds, c.events, c.entries, c.session, c.date, c.id)
	if e != nil {
		return ProofSummary{}, mapped(e)
	}
	card, e := c.cards.Read(c.ctx, c.seeds, c.events, c.entries, c.session, c.date)
	if e != nil {
		return ProofSummary{}, ErrSourceChanged
	}
	rawBout, _ := json.Marshal(bout.Record)
	rawCard, _ := json.Marshal(card.Record)
	source := r.Statement.Statement.Source
	if hash("SLUG candidate bout record\x00v1\x00", lp(rawBout)) != source.RecordDigest || hash("SLUG candidate card record\x00v1\x00", lp(rawCard)) != source.Source.CardRecordDigest || !sourceMatches(source, bout.Status) || card.Record.CardID != source.Source.CardID {
		return ProofSummary{}, ErrSourceChanged
	}
	tip, e := p.Observe(c.ctx)
	if e != nil {
		return ProofSummary{}, mapped(e)
	}
	resultTip := (*Tip)(nil)
	if r.Finality != nil {
		x := r.Finality.ObservedTip
		resultTip = &x
	}
	seedTip := card.Record.Source.Finality.ObservedTip
	refs := []seednode.HeaderRef{{Height: r.Inclusion.Anchor.Height, Hash: r.Inclusion.Anchor.Hash, Header: r.Inclusion.Anchor.Header}, {Height: r.Publication.Coin.BlockHeight, Hash: r.Publication.Coin.BlockHash}, {Height: card.Record.Source.Anchor.Height, Hash: card.Record.Source.Anchor.Hash, Header: card.Record.Source.Anchor.Header}}
	headers := map[string]ProofHeader{}
	var addPath func(seednode.HeaderRef) error
	addPath = func(ref seednode.HeaderRef) error {
		for {
			if c.ctx.Err() != nil {
				return ErrCancelled
			}
			if old, ok := headers[ref.Hash]; ok {
				if old.Height != ref.Height || ref.Header != "" && ref.Header != old.Header {
					return ErrProofUnavailable
				}
				return nil
			}
			if len(headers) >= 512 {
				return ErrCapacity
			}
			got, e := p.ExportHeader(c.ctx, ref)
			if e != nil {
				return ErrProofUnavailable
			}
			h, e := header(got.Header)
			if e != nil {
				return ErrProofUnavailable
			}
			headers[got.Hash] = ProofHeader{got.Height, got.Hash, got.Header}
			if got.Height == 0 {
				if got.Hash != s.meta.Pin.Genesis {
					return ErrProofUnavailable
				}
				return nil
			}
			ref = seednode.HeaderRef{Height: got.Height - 1, Hash: h.PrevBlock.String()}
		}
	}
	starts := append([]seednode.HeaderRef{}, refs...)
	starts = append(starts, seednode.HeaderRef{Height: tip.Height, Hash: tip.Hash}, seednode.HeaderRef{Height: seedTip.Height, Hash: seedTip.Hash})
	if resultTip != nil {
		starts = append(starts, seednode.HeaderRef{Height: resultTip.Height, Hash: resultTip.Hash})
	}
	for _, x := range starts {
		if e = addPath(x); e != nil {
			return ProofSummary{}, e
		}
	}
	ancestor := func(child string, height uint32, want string) bool {
		for {
			h, ok := headers[child]
			if !ok || h.Height < height {
				return false
			}
			if h.Height == height {
				return h.Hash == want
			}
			raw, e := header(h.Header)
			if e != nil {
				return false
			}
			child = raw.PrevBlock.String()
		}
	}
	a := r.Inclusion.Anchor
	coin := r.Publication.Coin
	if !ancestor(a.Hash, coin.BlockHeight, coin.BlockHash) || a.Height < coin.BlockHeight || coin.Coinbase && a.Height-coin.BlockHeight < 100 {
		return ProofSummary{}, ErrProofUnavailable
	}
	if r.Finality != nil && !ancestor(resultTip.Hash, a.Height, a.Hash) {
		return ProofSummary{}, ErrProofUnavailable
	}
	if !ancestor(seedTip.Hash, card.Record.Source.Target.Height, card.Record.Source.Target.Hash) || !ancestor(card.Record.Source.Target.Hash, card.Record.Source.Anchor.Height, card.Record.Source.Anchor.Hash) {
		return ProofSummary{}, ErrProofUnavailable
	}
	if r.Observation.Tip != nil {
		if h, ok := headers[r.Observation.Tip.Hash]; ok {
			if h.Height != r.Observation.Tip.Height || r.Observation.CurrentFinality && (!ancestor(h.Hash, a.Height, a.Hash) || h.Height < a.Height || h.Height-a.Height != r.Observation.Descendants) {
				return ProofSummary{}, ErrProofUnavailable
			}
		}
	}
	hs := make([]ProofHeader, 0, len(headers))
	for _, h := range headers {
		hs = append(hs, h)
	}
	sort.Slice(hs, func(i, j int) bool {
		if hs[i].Height == hs[j].Height {
			return hs[i].Hash < hs[j].Hash
		}
		return hs[i].Height < hs[j].Height
	})
	unique := map[string]seednode.HeaderRef{}
	for _, x := range refs {
		h := headers[x.Hash]
		x.Header = h.Header
		unique[x.Hash] = x
	}
	blocks := []seednode.HeaderRef{}
	for _, x := range unique {
		blocks = append(blocks, x)
	}
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].Height == blocks[j].Height {
			return blocks[i].Hash < blocks[j].Hash
		}
		return blocks[i].Height < blocks[j].Height
	})
	proof := PublicProof{Version: 1, Scope: "candidate_result_proof_v1", Pin: s.meta.Pin, Result: r, Status: st, Bout: bout.Record, Card: card.Record, ObservedTip: tip, ResultFinalityTip: resultTip, SourceFinalityTip: seedTip, Headers: hs, Blocks: nil}
	prefix, e := json.Marshal(proof)
	if e != nil || !bytes.HasSuffix(prefix, []byte(`"blocks":null}`)) {
		return ProofSummary{}, ErrCorrupt
	}
	prefix = append(prefix[:len(prefix)-len(`null}`)], '[')
	w := &proofWriter{dst: dst, h: sha256.New()}
	if _, e = w.Write(prefix); e != nil {
		return ProofSummary{}, e
	}
	payload, _ := Carrier(r.Statement.Statement)
	for i, ref := range blocks {
		raw, e := p.ExportBlock(c.ctx, ref)
		if e != nil {
			return ProofSummary{}, ErrProofUnavailable
		}
		var block wire.MsgBlock
		if block.Deserialize(bytes.NewReader(raw)) != nil {
			return ProofSummary{}, ErrProofUnavailable
		}
		if ref.Hash == a.Hash {
			got, e := anchorAt(&block, a.Height, a.Hash, a.TxIndex, 0, payload)
			if e != nil || !equal(got, a) {
				return ProofSummary{}, ErrProofUnavailable
			}
			var tx bytes.Buffer
			if block.Transactions[a.TxIndex].Serialize(&tx) != nil || !bytes.Equal(tx.Bytes(), rawID(r.Publication.RawTx)) {
				return ProofSummary{}, ErrProofUnavailable
			}
		}
		if ref.Hash == coin.BlockHash {
			found := false
			for _, tx := range block.Transactions {
				if tx.TxHash().String() != coin.Outpoint.TxID {
					continue
				}
				if int(coin.Outpoint.Index) >= len(tx.TxOut) {
					return ProofSummary{}, ErrProofUnavailable
				}
				out := tx.TxOut[coin.Outpoint.Index]
				if out.Value != coin.ValueSatoshis || !bytes.Equal(out.PkScript, rawID(coin.Script)) || coinbase(tx) != coin.Coinbase {
					return ProofSummary{}, ErrProofUnavailable
				}
				found = true
			}
			if !found {
				return ProofSummary{}, ErrProofUnavailable
			}
		}
		if ref.Hash == card.Record.Source.Anchor.Hash {
			old := card.Record.Source.Anchor
			var carrier [73]byte
			copy(carrier[:], rawID(old.Payload))
			got, e := anchorAt(&block, old.Height, old.Hash, old.TxIndex, old.OutputIndex, carrier)
			if e != nil || !equal(got, old) {
				return ProofSummary{}, ErrProofUnavailable
			}
		}
		body, e := json.Marshal(ProofBlock{ref.Height, ref.Hash, hex.EncodeToString(raw)})
		if e != nil {
			return ProofSummary{}, ErrCorrupt
		}
		if i > 0 {
			if _, e = w.Write([]byte{','}); e != nil {
				return ProofSummary{}, e
			}
		}
		if _, e = w.Write(body); e != nil {
			return ProofSummary{}, e
		}
	}
	if _, e = w.Write([]byte("]}\n")); e != nil {
		return ProofSummary{}, e
	}
	if e = bracket(c.ctx, p, tip, nil); e != nil {
		return ProofSummary{}, e
	}
	summary := ProofSummary{Version: 1, ResultKey: r.ResultKey, CardID: source.Source.CardID, BoutID: c.id, RecordDigest: recordDigest(r), SourceRecordDigest: source.RecordDigest, InputID: source.InputID, ResultDigest: source.ResultDigest, TxID: r.Publication.TxID, SHA256: hex.EncodeToString(w.h.Sum(nil)), Bytes: w.n, HeaderCount: uint32(len(hs)), BlockCount: uint32(len(blocks)), ObservedTip: tip}
	if r.Finality != nil {
		summary.FinalizedAtMS = r.Finality.ObservedAtMS
	}
	publish, e := prepare(summary)
	if e != nil {
		return ProofSummary{}, e
	}
	if publish == nil {
		return ProofSummary{}, ErrInvalid
	}
	if e = s.publishProof(c, source, st.Source, summary, publish); e != nil {
		return ProofSummary{}, e
	}
	return summary, nil
}

func (s *Store) publishProof(c sourceCall, source gymbout.ResultSource, status gymbout.Status, summary ProofSummary, publish func(func() error) error) error {
	published := false
	e := s.sourceView(c, gymaccount.View, func(v gymbout.ResultView, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := s.guard(c, check); e != nil {
			return e
		}
		if v.Source == nil || !equal(*v.Source, source) || !proofSourceSame(status, v.Status) {
			return ErrSourceChanged
		}
		row, ok, e := s.record(c.key)
		if e != nil {
			return e
		}
		if !ok || recordDigest(row) != summary.RecordDigest {
			return ErrSourceChanged
		}
		guard := func() error { return s.guard(c, check) }
		// The trusted callback owns its file mutation classification. A definite
		// pre-link refusal stays opaque; a possibly linked/synced failure must
		// already carry that callback's unknown outcome. Source unknown wins.
		if e = publish(guard); e != nil {
			return e
		}
		published = true
		return guard()
	})
	return s.finish(c, e, published)
}
