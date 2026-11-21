//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
)

type issueOracleCase struct {
	Name  string          `json:"name"`
	Valid bool            `json:"valid"`
	Value json.RawMessage `json:"value"`
	Hex   string          `json:"canonical_hex"`
}
type issueOracle struct {
	Fixtures struct {
		Wrappers  []issueOracleCase `json:"wrappers"`
		Transport []struct {
			Name   string                                   `json:"name"`
			Client poolservice.RecoverableDailyClientConfig `json:"client"`
			Worker RecoverablePoolConfig                    `json:"worker"`
		} `json:"conditional_transport"`
		Families []struct {
			Profile registry.IssueProfile `json:"profile"`
			Work    poolshare.MiningWork  `json:"work"`
			Setup   struct {
				Pool poolshare.Config `json:"pool"`
			} `json:"setup"`
		} `json:"matching_families"`
	} `json:"fixtures"`
	Outbox struct {
		Requests []issueOracleCase `json:"request_rows"`
		Cases    []issueOracleCase `json:"cases"`
		Entries  []dailyPoolCase   `json:"inherited_entry_anchors"`
	} `json:"outbox"`
}

func issueOracleRead(t *testing.T) issueOracle {
	t.Helper()
	raw, e := os.ReadFile("../testdata/issue-recovery/reference-v1.json")
	if e != nil || len(raw) != 6134980 {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "4bbe4eb9556ed5a894cee6ff565b0dcca805f27fea2d77cfe3a56242dc3dac91" {
		t.Fatal("reference identity")
	}
	var r issueOracle
	if json.Unmarshal(raw, &r) != nil || len(r.Outbox.Requests) != 6 || len(r.Fixtures.Transport) != 3 {
		t.Fatal("reference shape")
	}
	return r
}
func issueHex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func issueClose(t *testing.T, b *PoolOutbox) {
	t.Helper()
	if b != nil {
		if e := b.Close(); e != nil {
			t.Error(e)
		}
	}
}
func issueTestConfig(t *testing.T, c poolservice.RecoverableDailyClientConfig, base PoolConfig) (RecoverablePoolConfig, *poolservice.Client) {
	t.Helper()
	base.Source = c.Base.Source
	base.PublicKey = c.Base.PublicKey
	base.AuthorityOrigin = c.Base.URL
	base.AdmissionOrigin = c.Base.URL
	var e error
	base.TransportTrustID, e = poolservice.DailyTransportTrustID(c.Base)
	if e != nil {
		t.Fatal(e)
	}
	base.AdmissionTrustID, e = prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: c.Base.URL, Token: c.Base.Token, CACertificate: c.Base.CACertificate, AuthorityPin: c.Base.Source.AuthorityPin})
	if e != nil {
		t.Fatal(e)
	}
	trust, e := poolservice.RecoverableDailyTransportTrustID(c)
	if e != nil {
		t.Fatal(e)
	}
	selected := RecoverablePoolConfig{1, base, c.IssueProfile, c.Base.SettlementConfigID, c.Base.RecipientScript, trust}
	client, e := poolservice.NewRecoverableDailyClient(c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(client.Close)
	return selected, client
}
func TestRecoverablePoolIndependentConfigRowsAndModes(t *testing.T) {
	r := issueOracleRead(t)
	for _, v := range r.Fixtures.Transport {
		if _, e := ValidateRecoverableDailyPoolConfig(v.Worker); e != nil {
			t.Fatal(v.Name, e)
		}
	}
	for _, v := range r.Fixtures.Wrappers {
		if strings.HasSuffix(v.Name, "-worker-config") {
			var c RecoverablePoolConfig
			if poolDecodeBounded(issueHex(t, v.Hex), poolIssueMetadataLimit, &c, 14) != nil {
				t.Fatal(v.Name)
			}
			if _, e := ValidateRecoverableDailyPoolConfig(c); e != nil {
				t.Fatal(v.Name, e)
			}
		}
		if strings.HasSuffix(v.Name, "-worker-marker") {
			var m poolIssueMarker
			if poolDecodeBounded(issueHex(t, v.Hex), poolIssueMetadataLimit, &m, 14) != nil || m.Application != poolIssueApplication || m.Version != 1 {
				t.Fatal(v.Name)
			}
		}
	}
	// q cases use the independently matching one-enrollment family, not an
	// inherited transport fixture whose opaque SetupID describes another owner.
	var q poolIssueRow
	json.Unmarshal(r.Outbox.Requests[1].Value, &q)
	cc := r.Fixtures.Transport[0].Client
	cc.IssueProfile = r.Fixtures.Families[0].Profile
	cc.Base.Source = r.Fixtures.Families[0].Setup.Pool
	cc.Base.PublicKey = r.Fixtures.Families[0].Work.Job.PublicKey
	cc.Base.SettlementConfigID = q.Result.SettlementConfigID
	cc.Base.RecipientScript = q.Result.RecipientScript
	c, client := issueTestConfig(t, cc, r.Fixtures.Transport[0].Worker.Base)
	_ = client
	b := &PoolOutbox{config: c.Base, recovery: &poolIssueRecovery{config: c}}
	for _, v := range r.Outbox.Requests {
		t.Run(v.Name, func(t *testing.T) {
			var row poolIssueRow
			json.Unmarshal(v.Value, &row)
			_, e := b.decodeIssueRow(issueHex(t, v.Hex), row.Request.RequestID)
			if (e == nil) != v.Valid {
				t.Fatal("ordered retained response", e)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "outbox")
	box, e := CreateRecoverableDailyPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	issueClose(t, box)
	for _, open := range []func(string, PoolConfig) (*PoolOutbox, error){OpenPoolOutbox, OpenDailyPoolOutbox} {
		old, e := open(path, c.Base)
		issueClose(t, old)
		if e == nil {
			t.Fatal("old format adopted selected root")
		}
	}
	changed := c
	changed.Base.MaxRecords++
	other, e := OpenRecoverableDailyPoolOutbox(path, changed)
	issueClose(t, other)
	if e == nil {
		t.Fatal("limit increase adopted")
	}
	box, e = OpenRecoverableDailyPoolOutboxAudit(path, c)
	if e != nil {
		t.Fatal(e)
	}
	defer issueClose(t, box)
	if _, e = NewRecoverableDailyPool(c, client, client, &pwAdmission{c: c.Base}, &pwNode{}, issueNoSign, box); e == nil {
		t.Fatal("readonly outbox mined")
	}
}
func issueNoSign(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
	return gymauth.Authorization{}, errors.New("test does not sign")
}
func issueTestResponse(t *testing.T, c RecoverablePoolConfig, id string, w poolshare.MiningWork) poolservice.IssueResponse {
	t.Helper()
	pid, e := registry.IssueProfileID(c.IssueProfile)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(w.Template)
	sum := sha256.Sum256(raw)
	iid, e := registry.IssueIntentID(registry.IssueIntent{Version: 1, ProfileID: pid, RequestID: id, GymID: w.Job.GymID, PublicKey: w.Job.PublicKey, TemplateSHA256: hex.EncodeToString(sum[:])})
	if e != nil {
		t.Fatal(e)
	}
	return poolservice.IssueResponse{Version: 3, BindingID: c.IssueProfile.SourceConfigID, SettlementConfigID: c.SettlementConfigID, RecipientScript: c.RecipientScript, IssueProfileID: pid, RequestID: id, GymID: w.Job.GymID, PublicKey: w.Job.PublicKey, Result: poolshare.IssueResult{Version: 1, State: "work", IntentID: iid, JobID: w.Job.ID, ResolvedAtMS: w.Job.IssuedAtMS, Work: &w}}
}
func TestRecoverablePoolRunUnknownRestartExactRequest(t *testing.T) {
	r := issueOracleRead(t)
	tc := r.Fixtures.Transport[2]
	old := dailyPoolOracle(t)
	work := dailyPoolWork(t, old)
	var mu sync.Mutex
	var ids []string
	phase := 0
	var c RecoverablePoolConfig
	var box *PoolOutbox
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, in *http.Request) {
		<-ready
		if in.URL.Path != "/pool/v3/issue" {
			t.Error("unexpected route", in.URL.Path)
			out.WriteHeader(400)
			return
		}
		var req poolIssueRequest
		if json.NewDecoder(in.Body).Decode(&req) != nil {
			t.Error("request")
			return
		}
		mu.Lock()
		ids = append(ids, req.RequestID)
		p := phase
		mu.Unlock()
		// The actual registered intent exists before this first external dispatch.
		box.mu.Lock()
		raw, e := box.db.Get(poolRequestKey(req.RequestID), nil)
		active := box.meta.ActiveRequestID
		box.mu.Unlock()
		if e != nil || active != req.RequestID {
			t.Error("network preceded synced intent")
		}
		var q poolIssueRow
		if json.Unmarshal(raw, &q) != nil || q.Result != nil {
			t.Error("not original pending request")
		}
		out.Header().Set("Content-Type", "application/json")
		if p == 0 {
			out.WriteHeader(503)
			_, _ = out.Write([]byte("{\"version\":3,\"error\":{\"code\":\"outcome_unknown\",\"outcome_unknown\":true}}\n"))
			return
		}
		_ = json.NewEncoder(out).Encode(issueTestResponse(t, c, req.RequestID, work))
	}))
	defer server.Close()
	tc.Client.Base.URL = server.URL
	tc.Client.Base.CACertificate = ""
	c, client := issueTestConfig(t, tc.Client, tc.Worker.Base)
	path := filepath.Join(t.TempDir(), "box")
	var e error
	box, e = CreateRecoverableDailyPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { issueClose(t, box) }()
	close(ready)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	w, e := NewRecoverableDailyPool(c, client, client, &pwAdmission{c: c.Base}, &pwNode{}, issueNoSign, box)
	if e != nil {
		t.Fatal(e)
	}
	w.wait = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	if e = w.Run(ctx); e != nil {
		t.Fatal(e)
	}
	cancel()
	if w.Status().Attempts != 0 || box.meta.ActiveRequestID == "" || box.meta.Requests != 1 {
		t.Fatal("unknown did not retain exact request")
	}
	id := box.meta.ActiveRequestID
	before, _ := box.db.Get(poolRequestKey(id), nil)
	issueClose(t, box)
	box, e = OpenRecoverableDailyPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := box.db.Get(poolRequestKey(id), nil)
	if !bytes.Equal(before, after) {
		t.Fatal("reopen rewrote intent")
	}
	mu.Lock()
	phase = 1
	mu.Unlock()
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	node := &pwNode{call: func(string) { cancel() }}
	w, e = NewRecoverableDailyPool(c, client, client, &pwAdmission{c: c.Base}, node, issueNoSign, box)
	if e != nil {
		t.Fatal(e)
	}
	pwClock(w, work)
	if e = w.Run(ctx); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	same := len(ids) == 2 && ids[0] == id && ids[1] == id
	mu.Unlock()
	if !same || box.meta.Requests != 1 || box.meta.ActiveRequestID != "" || box.cachedScan == nil || box.meta.FirstReserved == 0 || w.Status().Attempts != 0 {
		t.Fatal("resume changed identity, failed atomic scan or hashed after cancellation")
	}
	raw, _ := box.db.Get(poolRequestKey(id), nil)
	var q poolIssueRow
	if json.Unmarshal(raw, &q) != nil || q.Result == nil {
		t.Fatal("completed request lost")
	}
	if client.CheckIssueResponse(id, *q.Result) == nil {
		t.Fatal("disk DTO minted provenance")
	}
	if e = box.saveScan(nil); e != nil {
		t.Fatal(e)
	}
	if box.meta.FirstReserved != 0 {
		t.Fatal("cleared scan reserve")
	}
	saved, _ := box.db.Get(poolRequestKey(id), nil)
	if !bytes.Equal(saved, raw) {
		t.Fatal("scan clearing destroyed result")
	}
	issueClose(t, box)
	box, e = OpenRecoverableDailyPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	if box.meta.Requests != 1 || box.meta.Reserved != 0 || box.cachedScan != nil {
		t.Fatal("completed reopen")
	}
}

func TestRecoverablePoolIntentCancellationCapacityAndUnknownSync(t *testing.T) {
	r := issueOracleRead(t)
	tc := r.Fixtures.Transport[2]
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	tc.Client.Base.URL = server.URL
	tc.Client.Base.CACertificate = ""
	c, client := issueTestConfig(t, tc.Client, tc.Worker.Base)
	for _, mode := range []string{"capacity", "before-dispatch", "unknown-sync"} {
		t.Run(mode, func(t *testing.T) {
			selected := c
			if mode == "capacity" {
				selected.Base.MaxRecords = 1
			}
			box, e := CreateRecoverableDailyPoolOutbox(filepath.Join(t.TempDir(), "box"), selected)
			if e != nil {
				t.Fatal(e)
			}
			defer issueClose(t, box)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			w, e := NewRecoverableDailyPool(selected, client, client, &pwAdmission{c: selected.Base}, &pwNode{}, issueNoSign, box)
			if e != nil {
				t.Fatal(e)
			}
			if mode == "before-dispatch" {
				box.checkpoint = func(string) { cancel() }
			}
			if mode == "unknown-sync" {
				real := box.writeBatch
				box.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
					if e := real(b, o); e != nil {
						return e
					}
					return errors.New("lost sync acknowledgement")
				}
			}
			w.wait = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
			e = w.Run(ctx)
			if mode == "unknown-sync" {
				if !errors.Is(e, ErrPoisoned) {
					t.Fatal(e)
				}
			} else if e != nil {
				t.Fatal(e)
			}
			if calls.Load() != 0 || w.Status().Attempts != 0 {
				t.Fatal("refusal dispatched or hashed")
			}
			if mode == "before-dispatch" && (box.meta.Requests != 1 || box.meta.ActiveRequestID == "") {
				t.Fatal("canceled intent lost")
			}
		})
	}
}

func TestRecoverablePoolNamespaceRejectsMixedWrongLinks(t *testing.T) {
	r := issueOracleRead(t)
	tc := r.Fixtures.Transport[2]
	c, client := issueTestConfig(t, tc.Client, tc.Worker.Base)
	_ = client
	for _, mutation := range []string{"orphan-cursor", "extra-key", "wrong-counter", "null-scan", "foreign-money"} {
		t.Run(mutation, func(t *testing.T) {
			box, e := CreateRecoverableDailyPoolOutbox(filepath.Join(t.TempDir(), "box"), c)
			if e != nil {
				t.Fatal(e)
			}
			defer issueClose(t, box)
			id := strings.Repeat("a", 64)
			q := poolIssueRow{Version: 1, Request: poolIssueRequest{id}}
			raw, _ := json.Marshal(q)
			reserve, _ := poolActiveReserve(q)
			m := box.meta
			m.Requests = 1
			m.ActiveRequestID = id
			m.Bytes = uint64(len(raw))
			m.Reserved = reserve
			batch := new(leveldb.Batch)
			batch.Put(poolRequestKey(id), raw)
			if e = box.commit(batch, m); e != nil {
				t.Fatal(e)
			}
			switch mutation {
			case "orphan-cursor":
				e = box.db.Put([]byte("cursor"), cursor(0), nil)
			case "extra-key":
				e = box.db.Put([]byte("qbad"), raw, nil)
			case "wrong-counter":
				m.Requests++
				v, _ := box.encodeMetadata(m)
				e = box.db.Put([]byte("meta"), v, nil)
			case "null-scan":
				v, _ := json.Marshal(poolIssueScan{1, id})
				e = box.db.Put([]byte("scan"), v, nil)
			case "foreign-money":
				row := issueTestResponse(t, c, id, dailyPoolWork(t, dailyPoolOracle(t)))
				row.SettlementConfigID = strings.Repeat("b", 64)
				q.Result = &row
				v, _ := json.Marshal(q)
				e = box.db.Put(poolRequestKey(id), v, nil)
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = box.audit(); e == nil {
				t.Fatal("accepted corrupt namespace")
			}
		})
	}
}

func TestRecoverablePoolPendingHistoryAndActiveRequestAudit(t *testing.T) {
	r := issueOracleRead(t)
	tc := r.Fixtures.Transport[2]
	c, _ := issueTestConfig(t, tc.Client, tc.Worker.Base)
	var entry PoolEntry
	for _, v := range r.Outbox.Entries {
		if v.Valid {
			var candidate PoolEntry
			if json.Unmarshal(v.Value, &candidate) == nil && candidate.Fighter == nil && validatePoolEntry(candidate, c.Base) == nil {
				entry = candidate
				break
			}
		}
	}
	if entry.Work.Job.ID == "" {
		t.Fatal("no independent authenticated-shape pool-only entry")
	}
	box, e := CreateRecoverableDailyPoolOutbox(filepath.Join(t.TempDir(), "box"), c)
	if e != nil {
		t.Fatal(e)
	}
	defer issueClose(t, box)
	id, later := strings.Repeat("a", 64), strings.Repeat("b", 64)
	completed := issueTestResponse(t, c, id, entry.Work)
	q := poolIssueRow{1, poolIssueRequest{id}, &completed}
	qraw, _ := json.Marshal(q)
	active := poolIssueRow{Version: 1, Request: poolIssueRequest{later}}
	araw, _ := json.Marshal(active)
	held, _ := poolActiveReserve(active)
	entry.Sequence = 1
	eraw, _ := json.Marshal(entry)
	reserve := poolFutureReserve(entry)
	m := box.meta
	m.Requests = 2
	m.ActiveRequestID = later
	m.Records = 1
	m.LastSequence = 1
	m.Bytes = uint64(len(qraw) + len(araw) + len(eraw))
	m.Reserved = held + reserve
	batch := new(leveldb.Batch)
	batch.Put(poolRequestKey(id), qraw)
	batch.Put(poolRequestKey(later), araw)
	batch.Put(seqKey(1), eraw)
	batch.Put(hashKey(entry.DuplicateKey), seqKey(1))
	if !poolDone(entry) {
		m.Pending = 1
		batch.Put(poolPendingKey(entry), seqKey(1))
	}
	if e = box.commit(batch, m); e != nil {
		t.Fatal(e)
	}
	if e = box.audit(); e != nil {
		t.Fatal("original completed/pending entry plus distinct active issue", e)
	}
	// Entry result link is exact, not merely job/count/key shape.
	completed.Result.Work.Job.Challenge = strings.Repeat("c", 64)
	q.Result = &completed
	qraw, _ = json.Marshal(q)
	if e = box.db.Put(poolRequestKey(id), qraw, nil); e != nil {
		t.Fatal(e)
	}
	if e = box.audit(); e == nil {
		t.Fatal("modified retained work admitted")
	}
}

func TestRecoverablePoolTerminalClearsOnlyValidatedResult(t *testing.T) {
	r := issueOracleRead(t)
	old := dailyPoolOracle(t)
	work := dailyPoolWork(t, old)
	for _, state := range []string{"issued_unused", "not_issued"} {
		t.Run(state, func(t *testing.T) {
			var c RecoverablePoolConfig
			var observed string
			var observation sync.Mutex
			ready := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, in *http.Request) {
				<-ready
				var q poolIssueRequest
				_ = json.NewDecoder(in.Body).Decode(&q)
				observation.Lock()
				observed = q.RequestID
				observation.Unlock()
				response := issueTestResponse(t, c, q.RequestID, work)
				response.Result.State = state
				response.Result.Work = nil
				if state == "issued_unused" {
					response.Result.Reason = "expired_before_work"
					response.Result.ResolvedAtMS = work.Job.ExpiresAtMS
				} else {
					response.Result.JobID = ""
					response.Result.Reason = "intake_drained"
				}
				out.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(out).Encode(response)
			}))
			defer server.Close()
			tc := r.Fixtures.Transport[2]
			tc.Client.Base.URL = server.URL
			tc.Client.Base.CACertificate = ""
			var client *poolservice.Client
			c, client = issueTestConfig(t, tc.Client, tc.Worker.Base)
			close(ready)
			box, e := CreateRecoverableDailyPoolOutbox(filepath.Join(t.TempDir(), "box"), c)
			if e != nil {
				t.Fatal(e)
			}
			defer issueClose(t, box)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			w, e := NewRecoverableDailyPool(c, client, client, &pwAdmission{c: c.Base}, &pwNode{}, issueNoSign, box)
			if e != nil {
				t.Fatal(e)
			}
			w.cycleHook = func(context.Context) error {
				box.mu.Lock()
				done := box.meta.Requests == 1 && box.meta.ActiveRequestID == ""
				box.mu.Unlock()
				if done {
					cancel()
				}
				return nil
			}
			if e = w.Run(ctx); e != nil {
				t.Fatal(e)
			}
			observation.Lock()
			seen := observed
			observation.Unlock()
			if seen == "" || box.meta.Requests != 1 || box.meta.Reserved != 0 || box.meta.ActiveRequestID != "" || box.cachedScan != nil || w.Status().Attempts != 0 {
				t.Fatal("terminal minted work, lost row or allocated again")
			}
			if e = box.audit(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestRecoverablePoolRunReconcilesOriginalEntryBeforeResume(t *testing.T) {
	r := issueOracleRead(t)
	tc := r.Fixtures.Transport[2]
	var pending, accepted PoolEntry
	for _, v := range r.Outbox.Entries {
		if v.Name == "pool-only-pending" {
			if e := json.Unmarshal(v.Value, &pending); e != nil {
				t.Fatal(e)
			}
		}
		if v.Name == "pool-only-accepted" {
			if e := json.Unmarshal(v.Value, &accepted); e != nil {
				t.Fatal(e)
			}
		}
	}
	if accepted.PoolReceipt == nil {
		t.Fatal("missing independent exact receipt")
	}
	var c RecoverablePoolConfig
	var mu sync.Mutex
	var paths []string
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, in *http.Request) {
		<-ready
		mu.Lock()
		paths = append(paths, in.URL.Path)
		mu.Unlock()
		out.Header().Set("Content-Type", "application/json")
		switch in.URL.Path {
		case "/pool/v3/lookup":
			pid, _ := registry.IssueProfileID(c.IssueProfile)
			_ = json.NewEncoder(out).Encode(poolservice.RecoverableSubmissionResponse{Version: 3, BindingID: c.IssueProfile.SourceConfigID, SettlementConfigID: c.SettlementConfigID, RecipientScript: c.RecipientScript, IssueProfileID: pid, Submission: *accepted.PoolReceipt})
		case "/pool/v3/issue":
			out.WriteHeader(503)
			_, _ = out.Write([]byte("{\"version\":3,\"error\":{\"code\":\"outcome_unknown\",\"outcome_unknown\":true}}\n"))
		default:
			t.Error("unexpected effect", in.URL.Path)
			out.WriteHeader(400)
		}
	}))
	defer server.Close()
	tc.Client.Base.URL = server.URL
	tc.Client.Base.CACertificate = ""
	var client *poolservice.Client
	c, client = issueTestConfig(t, tc.Client, tc.Worker.Base)
	close(ready)
	path := filepath.Join(t.TempDir(), "box")
	box, e := CreateRecoverableDailyPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { issueClose(t, box) }()
	id, active := strings.Repeat("a", 64), strings.Repeat("b", 64)
	response := issueTestResponse(t, c, id, pending.Work)
	qraw, _ := json.Marshal(poolIssueRow{1, poolIssueRequest{id}, &response})
	q := poolIssueRow{Version: 1, Request: poolIssueRequest{active}}
	araw, _ := json.Marshal(q)
	reserve, _ := poolActiveReserve(q)
	eraw, _ := json.Marshal(pending)
	m := box.meta
	m.Requests = 2
	m.ActiveRequestID = active
	m.LastSequence = 1
	m.Records = 1
	m.Pending = 1
	m.Bytes = uint64(len(qraw) + len(araw) + len(eraw))
	m.Reserved = reserve + poolFutureReserve(pending)
	batch := new(leveldb.Batch)
	batch.Put(poolRequestKey(id), qraw)
	batch.Put(poolRequestKey(active), araw)
	batch.Put(seqKey(1), eraw)
	batch.Put(hashKey(pending.DuplicateKey), seqKey(1))
	batch.Put(poolPendingKey(pending), seqKey(1))
	if e = box.commit(batch, m); e != nil {
		t.Fatal(e)
	}
	issueClose(t, box)
	box, e = OpenRecoverableDailyPoolOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	w, e := NewRecoverableDailyPool(c, client, client, &pwAdmission{c: c.Base}, &pwNode{}, issueNoSign, box)
	if e != nil {
		t.Fatal(e)
	}
	w.now = func() time.Time { return time.UnixMilli(pending.Work.Job.IssuedAtMS + 10000) }
	w.wait = func(context.Context, time.Duration) error { cancel(); return ctx.Err() }
	if e = w.Run(ctx); e != nil {
		t.Fatal(e)
	}
	rows, e := box.List(0, 1)
	if e != nil || len(rows) != 1 || rows[0].PoolReceipt == nil || *rows[0].PoolReceipt != *accepted.PoolReceipt || rows[0].PoolState != "accepted" || box.meta.ActiveRequestID != active || box.meta.Requests != 2 || box.meta.Pending != 0 || w.Status().Attempts != 0 {
		t.Fatal("exact pending reconciliation or active intent changed", e)
	}
	mu.Lock()
	ordered := len(paths) == 2 && paths[0] == "/pool/v3/lookup" && paths[1] == "/pool/v3/issue"
	mu.Unlock()
	if !ordered {
		t.Fatal("pending recovery did not precede exact issue resume")
	}
	if e = box.audit(); e != nil {
		t.Fatal(e)
	}
}

type issueDTOAuthority struct {
	*poolservice.Client
	response poolservice.IssueResponse
}

func (a *issueDTOAuthority) ResumeIssue(context.Context, string) (poolservice.IssueResponse, error) {
	return a.response, nil
}
func TestRecoverablePoolRejectsManufacturedDTOAuthority(t *testing.T) {
	r := issueOracleRead(t)
	tc := r.Fixtures.Transport[2]
	c, client := issueTestConfig(t, tc.Client, tc.Worker.Base)
	box, e := CreateRecoverableDailyPoolOutbox(filepath.Join(t.TempDir(), "box"), c)
	if e != nil {
		t.Fatal(e)
	}
	defer issueClose(t, box)
	a := &issueDTOAuthority{Client: client}
	w, e := NewRecoverableDailyPool(c, client, a, &pwAdmission{c: c.Base}, &pwNode{}, issueNoSign, box)
	if e != nil {
		t.Fatal(e)
	}
	if e = w.Run(context.Background()); !errors.Is(e, ErrCorrupt) {
		t.Fatal("unproven response admitted", e)
	}
	if box.meta.Requests != 1 || box.meta.ActiveRequestID == "" || box.cachedScan != nil || w.Status().Attempts != 0 {
		t.Fatal("invalid response cleared durable request")
	}
	if _, e = NewDailyPool(c.Base, client, &pwAdmission{c: c.Base}, &pwNode{}, issueNoSign, box); e == nil {
		t.Fatal("old constructor borrowed selected owners")
	}
}
