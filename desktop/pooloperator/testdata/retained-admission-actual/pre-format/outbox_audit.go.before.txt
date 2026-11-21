// Public evidence only, after every original outbox owner has joined.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectworker"
)

// Scalar-only so equality compares every captured control and the complete
// logical namespace. RowsSHA256 includes keys and values, not just e/ rows.
type outboxState struct {
	Records           uint64 `json:"records"`
	Pending           uint64 `json:"pending"`
	Requests          uint64 `json:"requests"`
	ActiveRequestID   string `json:"active_request_id"`
	LogicalBytes      uint64 `json:"logical_bytes"`
	Reserved          uint64 `json:"reserved"`
	MetaSHA256        string `json:"meta_sha256"`
	MetaBytes         int    `json:"meta_bytes"`
	MarkerSHA256      string `json:"marker_sha256"`
	ScanSHA256        string `json:"scan_sha256"`
	ScanRequestID     string `json:"scan_request_id"`
	CursorSHA256      string `json:"cursor_sha256"`
	NextNonce         uint64 `json:"next_nonce"`
	RequestsSHA256    string `json:"requests_sha256"`
	RowsSHA256        string `json:"rows_sha256"`
	NamespaceRows     uint64 `json:"namespace_rows"`
	ReadOnlyUnchanged bool   `json:"read_only_unchanged"`
}

type recoveryOutboxMeta struct {
	Application     string                               `json:"application"`
	Version         uint32                               `json:"version"`
	Config          prospectworker.RecoverablePoolConfig `json:"config"`
	LastSequence    uint64                               `json:"last_sequence"`
	Records         uint64                               `json:"records"`
	Bytes           uint64                               `json:"bytes"`
	Pending         uint64                               `json:"pending"`
	Reserved        uint64                               `json:"reserved"`
	Requests        uint64                               `json:"requests"`
	ActiveRequestID string                               `json:"active_request_id"`
}
type recoveryOutboxRequest struct {
	Version uint32                     `json:"version"`
	Request poolservice.IssueRequest   `json:"request"`
	Result  *poolservice.IssueResponse `json:"result"`
}

func readOutbox(root string, c pilot.RecoverableRemotePoolConfig) ([]prospectworker.PoolEntry, outboxState) {
	rows, state, _ := readSelectedOutbox(root, c, "")
	return rows, state
}

func readRequest(root string, c pilot.RecoverableRemotePoolConfig, id string) (*poolservice.IssueResponse, outboxState) {
	raw, e := hex.DecodeString(id)
	require(e == nil && len(raw) == 32 && hx(raw) == id && !bytes.Equal(raw, make([]byte, 32)), "exact selected request ID")
	_, state, response := readSelectedOutbox(root, c, id)
	return response, state
}

func readSelectedOutbox(root string, c pilot.RecoverableRemotePoolConfig, requestID string) (rows []prospectworker.PoolEntry, state outboxState, response *poolservice.IssueResponse) {
	path := filepath.Join(root, "outbox")
	wc := recoveryWorkerConfig(c)
	before := outboxPublicFiles(path)
	b, e := prospectworker.OpenRecoverableDailyPoolOutboxAudit(path, wc)
	if e != nil {
		if b != nil {
			e = errors.Join(e, b.Close())
		}
		check(e, "closed original recoverable outbox audit")
	}
	closed := false
	defer func() {
		if !closed {
			check(b.Close(), "partial selected outbox audit close")
		}
	}()
	rows = []prospectworker.PoolEntry{}
	for {
		after := uint64(len(rows))
		page, re := b.List(after, 100)
		check(re, "original audited outbox page")
		if len(page) == 0 {
			break
		}
		require(len(rows)+len(page) <= 64, "finite actual retained entries")
		for i, row := range page {
			require(row.Sequence == after+uint64(i)+1, "complete original entry prefix")
		}
		rows = append(rows, page...)
	}
	e = b.Close()
	closed = true
	check(e, "selected original outbox audit joined")

	// This physical projection is reachable only after the concrete selected
	// audit validated all original q/scan/e/h/p linkage and closed successfully.
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true, Strict: opt.StrictAll})
	check(e, "readonly selected outbox projection")
	dbclosed := false
	defer func() {
		if !dbclosed {
			check(db.Close(), "partial selected projection close")
		}
	}()
	state, response = projectSelectedOutbox(db, wc, rows, requestID)
	e = db.Close()
	dbclosed = true
	check(e, "selected outbox projection joined")
	require(reflect.DeepEqual(before, outboxPublicFiles(path)), "selected outbox audits changed no public files")
	state.MarkerSHA256 = before["SLUG-POOL-WORKER.json"]
	require(len(state.MarkerSHA256) == 64, "original selected marker retained")
	state.ReadOnlyUnchanged = true
	return
}

func projectSelectedOutbox(db *leveldb.DB, c prospectworker.RecoverablePoolConfig, rows []prospectworker.PoolEntry, requestID string) (state outboxState, response *poolservice.IssueResponse) {
	raw, e := db.Get([]byte("meta"), nil)
	check(e, "original audited request metadata")
	require(len(raw) > 0 && len(raw) <= 256<<10, "selected metadata bound")
	var m recoveryOutboxMeta
	check(json.Unmarshal(raw, &m), "original selected metadata projection")
	canonical, e := json.Marshal(m)
	check(e, "selected metadata canonical bytes")
	require(bytes.Equal(canonical, raw) && m.Application == "slug-pool-worker-issue-recovery" && m.Version == 1 && m.Config == c && m.LastSequence == m.Records && m.Records == uint64(len(rows)) && m.Pending <= m.Records, "exact selected metadata/config/counts")
	require(m.Requests <= c.IssueProfile.MaxRequests && m.Requests <= c.Base.MaxRecords && m.Records <= c.Base.MaxRecords-m.Requests && m.Bytes <= c.Base.MaxBytes && m.Reserved <= c.Base.MaxBytes-m.Bytes, "original request/entry reservation bounds")
	state = outboxState{Records: m.Records, Pending: m.Pending, Requests: m.Requests, ActiveRequestID: m.ActiveRequestID, LogicalBytes: m.Bytes, Reserved: m.Reserved, MetaSHA256: hx(digest(raw)), MetaBytes: len(raw)}
	all, requests := sha256.New(), sha256.New()
	all.Write([]byte("SLUG actual recoverable outbox namespace\x00v1\x00"))
	requests.Write([]byte("SLUG actual recoverable outbox requests\x00v1\x00"))
	frame := func(h io.Writer, k, v []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(k)))
		_, _ = h.Write(n[:])
		_, _ = h.Write(k)
		binary.BigEndian.PutUint64(n[:], uint64(len(v)))
		_, _ = h.Write(n[:])
		_, _ = h.Write(v)
	}
	var entries, indexes, pending, countQ, nulls, payload uint64
	found := requestID == ""
	it := db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		k, v := it.Key(), it.Value()
		state.NamespaceRows++
		require(state.NamespaceRows <= 3+3*c.Base.MaxRecords+c.IssueProfile.MaxRequests, "finite selected namespace")
		switch {
		case bytes.Equal(k, []byte("meta")):
			require(bytes.Equal(v, raw), "same projected metadata")
		case bytes.Equal(k, []byte("scan")):
			var scan struct {
				Version   uint32 `json:"version"`
				RequestID string `json:"request_id"`
			}
			require(len(v) > 0 && len(v) <= 256, "selected scan reference bound")
			check(json.Unmarshal(v, &scan), "original scan reference")
			require(scan.Version == 1 && len(scan.RequestID) == 64, "selected scan ID")
			state.ScanSHA256 = hx(digest(v))
			state.ScanRequestID = scan.RequestID
			payload += uint64(len(v))
		case bytes.Equal(k, []byte("cursor")):
			require(len(v) == 8, "original cursor bytes")
			state.NextNonce = binary.BigEndian.Uint64(v)
			require(state.NextNonce <= 1<<32, "original nonce cursor")
			state.CursorSHA256 = hx(digest(v))
			payload += 8
		case len(k) == 9 && k[0] == 'e':
			entries++
			require(entries <= uint64(len(rows)) && binary.BigEndian.Uint64(k[1:]) == entries && len(v) <= 40<<20, "original bounded entry")
			b, e := json.Marshal(rows[entries-1])
			check(e, "owned entry projection")
			require(bytes.Equal(b, v), "same full audited entry")
			payload += uint64(len(v))
		case len(k) == 65 && k[0] == 'h':
			indexes++
			require(len(v) == 9 && v[0] == 'e', "original duplicate index")
		case len(k) == 17 && k[0] == 'p':
			pending++
			require(len(v) == 9 && v[0] == 'e', "original pending index")
		case len(k) == 33 && k[0] == 'q':
			countQ++
			require(countQ <= m.Requests && len(v) > 0 && len(v) <= poolservice.MaxWorkResponseSize+256, "bounded retained request")
			id := hx(k[1:])
			var q recoveryOutboxRequest
			check(json.Unmarshal(v, &q), "original retained request")
			b, e := json.Marshal(q)
			check(e, "request canonical bytes")
			require(bytes.Equal(b, v) && q.Version == 1 && q.Request.RequestID == id, "exact request key and bytes")
			if q.Result == nil {
				nulls++
				require(id == m.ActiveRequestID, "original null request active")
			} else {
				selection := poolservice.RecoverableDailySelection{Version: 1, BaseVersion: 2, URL: c.Base.AuthorityOrigin, PublicKey: c.Base.PublicKey, Source: c.Base.Source, IssueProfile: c.IssueProfile, SettlementConfigID: c.SettlementConfigID, RecipientScript: c.RecipientScript, BaseTransportTrustID: c.Base.TransportTrustID, TransportTrustID: c.TransportTrustID}
				check(poolservice.ValidateRecoverableIssueResponse(selection, id, *q.Result, false), "complete persisted selected result; no provenance minted")
			}
			if id == requestID {
				found = true
				response = q.Result
			}
			frame(requests, k, v)
			payload += uint64(len(v))
		default:
			panic("unexpected selected outbox namespace")
		}
		require(payload <= c.Base.MaxBytes, "finite exact logical payload")
		frame(all, k, v)
	}
	check(it.Error(), "complete selected namespace iterator")
	wantNull := uint64(0)
	if m.ActiveRequestID != "" {
		wantNull = 1
	}
	require(found && entries == m.Records && indexes == entries && pending == m.Pending && countQ == m.Requests && nulls == wantNull && payload == m.Bytes, "complete selected request/entry/index census")
	require((state.ScanSHA256 == "") == (state.CursorSHA256 == "") && (m.ActiveRequestID == "" || state.ScanSHA256 == ""), "original active request and scan exclusive")
	state.RowsSHA256 = hx(all.Sum(nil))
	state.RequestsSHA256 = hx(requests.Sum(nil))
	return
}

// This inventory is restricted to the selected public outbox, never its sibling
// wallet, vault or credential files. Bounds describe this finite acceptance only.
func outboxPublicFiles(root string) map[string]string {
	out := map[string]string{}
	var total int64
	check(filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		require(info.Mode().IsRegular() && info.Size() <= 128<<20 && len(out) < 4096, "bounded public outbox inventory")
		total += info.Size()
		require(total <= 512<<20, "finite public outbox inventory bytes")
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		opened, se := f.Stat()
		h := sha256.New()
		n, re := io.Copy(h, io.LimitReader(f, (128<<20)+1))
		ce := f.Close()
		if e = errors.Join(se, re, ce); e != nil {
			return e
		}
		require(os.SameFile(info, opened) && n == info.Size(), "same original public outbox file")
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		out[rel] = hx(h.Sum(nil))
		return nil
	}), "public outbox inventory")
	return out
}
