// Exact selected TLS forwarding; only the original target admission is held/lost.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type admissionTarget struct {
	RequestID      string                                  `json:"request_id"`
	Work           poolshare.MiningWork                    `json:"work"`
	Proof          prospect.Proof                          `json:"proof"`
	Authorization  gymauth.Authorization                   `json:"authorization"`
	Receipt        poolshare.Submission                    `json:"receipt"`
	FirstReceipt   poolshare.Submission                    `json:"first_receipt"`
	FirstAdmission prospectservice.ManifestAdmissionResult `json:"first_admission"`
	BodySHA256     string                                  `json:"admission_body_sha256"`
	FirstHeldAtMS  int64                                   `json:"first_held_at_ms"`
}
type admissionRelayEvent struct {
	Ordinal         uint64                                   `json:"ordinal"`
	Path            string                                   `json:"path"`
	StartedAtMS     int64                                    `json:"started_at_ms"`
	EndedAtMS       int64                                    `json:"ended_at_ms"`
	RequestID       string                                   `json:"request_id"`
	JobID           string                                   `json:"job_id"`
	Header          string                                   `json:"header"`
	RequestBytes    int                                      `json:"request_bytes"`
	RequestSHA256   string                                   `json:"request_sha256"`
	Target          bool                                     `json:"target"`
	Mode            string                                   `json:"mode"`
	HeldAtMS        int64                                    `json:"held_at_ms"`
	HoldDeadlineMS  int64                                    `json:"hold_deadline_ms"`
	ReleasedAtMS    int64                                    `json:"released_at_ms"`
	ForwardedAtMS   int64                                    `json:"forwarded_at_ms"`
	Status          int                                      `json:"status"`
	ResponseBytes   int                                      `json:"response_bytes"`
	ResponseSHA256  string                                   `json:"response_sha256"`
	Lost            bool                                     `json:"lost"`
	DownstreamBytes int                                      `json:"downstream_bytes"`
	Context         string                                   `json:"context"`
	Failure         string                                   `json:"failure"`
	Admission       *prospectservice.ManifestAdmissionResult `json:"admission"`
	Error           *relayAPIError                           `json:"api_error"`
}
type relayAPIError struct {
	Version uint32 `json:"version"`
	Error   struct {
		Code           string `json:"code"`
		OutcomeUnknown bool   `json:"outcome_unknown"`
	} `json:"error"`
}
type admissionRelayEvidence struct {
	Version        uint32                                   `json:"version"`
	MaxEvents      int                                      `json:"max_events"`
	Target         *admissionTarget                         `json:"target"`
	Mode           string                                   `json:"mode"`
	Lost           bool                                     `json:"lost"`
	LostResponse   *prospectservice.ManifestAdmissionResult `json:"lost_response"`
	Events         []admissionRelayEvent                    `json:"events"`
	Failure        bool                                     `json:"failure"`
	Closed         bool                                     `json:"closed"`
	HandlersJoined bool                                     `json:"handlers_joined"`
}
type admissionTLSRelay struct {
	server                          *http.Server
	listener                        net.Listener
	client                          *http.Client
	done                            chan error
	active                          sync.WaitGroup
	mu                              sync.Mutex
	closing, closed, closeAttempted bool
	closeErr, failure               error
	expected                        poolservice.RecoverableDailySelection
	works                           map[string]poolservice.IssueResponse
	receipts                        map[uint64]poolshare.Submission
	admitted                        map[string]prospectservice.ManifestAdmissionResult
	target                          *admissionTarget
	mode                            string
	targetActive                    int
	handlers                        int
	held                            chan struct{}
	release                         chan struct{}
	heldNotified, released, lost    bool
	loss                            chan struct{}
	result                          chan struct{}
	resultSent                      bool
	lastResponse                    *prospectservice.ManifestAdmissionResult
	lastError                       *relayAPIError
	lostResponse                    *prospectservice.ManifestAdmissionResult
	events                          []admissionRelayEvent
}

func publicSelection(c poolservice.RecoverableDailyClientConfig) poolservice.RecoverableDailySelection {
	b, e := poolservice.DailyTransportTrustID(c.Base)
	check(e, "base TLS trust")
	id, e := poolservice.RecoverableDailyTransportTrustID(c)
	check(e, "selected TLS trust")
	return poolservice.RecoverableDailySelection{Version: 1, BaseVersion: 2, URL: c.Base.URL, PublicKey: c.Base.PublicKey, Source: c.Base.Source, IssueProfile: c.IssueProfile, SettlementConfigID: c.Base.SettlementConfigID, RecipientScript: c.Base.RecipientScript, BaseTransportTrustID: b, TransportTrustID: id}
}
func canonicalPublic(raw []byte, cap int, v any) bool {
	if len(raw) == 0 || len(raw) > cap {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return false
	}
	b, e := json.Marshal(v)
	return e == nil && bytes.Equal(bytes.Trim(raw, " \t\r\n"), b)
}
func admissionKey(job, header string) string { return job + ":" + header }
func relayContext(e error) string {
	if e == nil {
		return ""
	}
	if errors.Is(e, context.Canceled) {
		return "canceled"
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	return "other"
}

func startAdmissionTLS(ctx context.Context, private, backend string, c poolservice.RecoverableDailyClientConfig) *admissionTLSRelay {
	selected, e := url.Parse(c.Base.URL)
	check(e, "selected TLS URL")
	cert, e := tls.LoadX509KeyPair(filepath.Join(private, "tls-cert.pem"), filepath.Join(private, "tls-key.pem"))
	check(e, "original TLS keypair")
	roots := x509.NewCertPool()
	require(roots.AppendCertsFromPEM([]byte(c.Base.CACertificate)), "selected CA")
	l, e := net.Listen("tcp4", selected.Host)
	check(e, "original selected Origin listener")
	tr := &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 65536, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, RootCAs: roots, ServerName: selected.Hostname()}}
	r := &admissionTLSRelay{listener: tls.NewListener(l, &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, NextProtos: []string{"http/1.1"}}), client: &http.Client{Timeout: 30 * time.Second, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, done: make(chan error, 1), expected: publicSelection(c), works: map[string]poolservice.IssueResponse{}, receipts: map[uint64]poolshare.Submission{}, admitted: map[string]prospectservice.ManifestAdmissionResult{}, mode: "retain", held: make(chan struct{}), release: make(chan struct{}), loss: make(chan struct{}), result: make(chan struct{}), events: []admissionRelayEvent{}}
	r.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 35 * time.Second, MaxHeaderBytes: 16384, BaseContext: func(net.Listener) context.Context { return ctx }}
	r.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) { r.forward(w, q, backend, selected.Host) })
	go func() { r.done <- r.server.Serve(r.listener) }()
	return r
}
func (r *admissionTLSRelay) locked(f func()) { r.mu.Lock(); defer r.mu.Unlock(); f() }
func (r *admissionTLSRelay) fail(e error) {
	if e != nil {
		r.mu.Lock()
		r.failure = errors.Join(r.failure, e)
		r.mu.Unlock()
	}
}
func (r *admissionTLSRelay) forward(w http.ResponseWriter, q *http.Request, backend, host string) {
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		http.Error(w, "closed", 503)
		return
	}
	r.active.Add(1)
	r.handlers++
	r.mu.Unlock()
	defer r.active.Done()
	index := -1
	ev := admissionRelayEvent{Path: q.URL.Path, StartedAtMS: time.Now().UnixMilli()}
	targetActive := false
	defer func() {
		if p := recover(); p != nil {
			ev.Failure = "validation"
			r.fail(fmt.Errorf("bounded relay validation: %.256s", p))
			http.Error(w, "relay validation", 502)
		}
		ev.EndedAtMS = time.Now().UnixMilli()
		ev.Context = relayContext(q.Context().Err())
		r.mu.Lock()
		if index >= 0 {
			r.events[index] = ev
		}
		if targetActive {
			r.targetActive--
		}
		r.handlers--
		r.mu.Unlock()
	}()
	require(q.Host == host && q.URL.RawQuery == "" && q.URL.Fragment == "", "exact relay Origin")
	body, e := io.ReadAll(io.LimitReader(q.Body, poolservice.MaxWorkResponseSize+1))
	check(e, "bounded original request read")
	require(len(body) <= poolservice.MaxWorkResponseSize, "original request bound")
	tracked := q.URL.Path == "/v3/admissions" || q.URL.Path == "/pool/v3/submit" || q.URL.Path == "/pool/v3/lookup" || q.URL.Path == "/pool/v3/issue" || q.URL.Path == "/pool/v3/issue-lookup"
	if tracked {
		r.locked(func() {
			require(len(r.events) < 512, "relay event bound")
			index = len(r.events)
			ev.Ordinal = uint64(index + 1)
			r.events = append(r.events, ev)
		})
		ev.RequestBytes = len(body)
		ev.RequestSHA256 = hx(digest(body))
		require(q.Method == http.MethodPost, "original tracked POST")
	}
	var proof prospect.Proof
	var auth gymauth.Authorization
	var poolRequest struct {
		JobID string `json:"job_id"`
		Nonce uint32 `json:"nonce"`
	}
	if q.URL.Path == "/v3/admissions" {
		proof, auth, e = prospectservice.ParseSubmission(body)
		check(e, "original signed admission grammar")
		ev.JobID, ev.Header = proof.JobID, proof.Header
		r.mu.Lock()
		second, hasSecond := r.receipts[2]
		selected := hasSecond && proof.JobID == second.JobID && proof.Header == second.Header
		var work poolservice.IssueResponse
		var first poolshare.Submission
		var firstAdmission prospectservice.ManifestAdmissionResult
		if selected {
			work = r.works[proof.JobID]
			first = r.receipts[1]
			firstAdmission = r.admitted[admissionKey(first.JobID, first.Header)]
		}
		r.mu.Unlock()
		if selected {
			require(work.Result.Work != nil && first.AcceptedSequence == 1 && second.AcceptedSequence == 2, "target exact second original receipt")
			require(firstAdmission.Inserted && firstAdmission.Record.Job.ID == first.JobID && firstAdmission.Record.Verified.WorkHash == first.WorkHash, "first pool winner admitted normally before target")
			check(gymauth.VerifyAuthorization(work.Result.Work.Job.Context, proof, auth), "original target signature")
			verified, ve := prospect.Verify(work.Result.Work.Job.Context, proof)
			check(ve, "original target normal proof")
			require(verified.WorkHash == second.WorkHash && verified.GymID == second.GymID && second.Pool && second.Block, "target exact canonical winning work")
			var mode string
			var release chan struct{}
			hold := false
			r.locked(func() {
				if r.target == nil {
					r.target = &admissionTarget{work.RequestID, *work.Result.Work, proof, auth, second, first, firstAdmission, ev.RequestSHA256, time.Now().UnixMilli()}
				}
				target := r.target
				mode = r.mode
				release = r.release
				require(reflect.DeepEqual(target.Proof, proof) && target.Authorization == auth && target.BodySHA256 == ev.RequestSHA256, "exact retained signed request bytes")
				r.targetActive++
				targetActive = true
				ev.Target = true
				ev.Mode = mode
				ev.RequestID = target.RequestID
				hold = mode == "retain" || mode == "loss" || mode == "hold-after-loss"
				if hold {
					now := time.Now()
					ev.HeldAtMS = now.UnixMilli()
					deadline := now.Add(10 * time.Second)
					if expiry := time.UnixMilli(target.Work.Job.ExpiresAtMS); expiry.Before(deadline) {
						deadline = expiry
					}
					if d, ok := q.Context().Deadline(); ok && d.Before(deadline) {
						deadline = d
					}
					ev.HoldDeadlineMS = deadline.UnixMilli()
					if !r.heldNotified {
						r.heldNotified = true
						close(r.held)
					}
				}
			})
			if hold {
				holdContext, cancel := context.WithDeadline(q.Context(), time.UnixMilli(ev.HoldDeadlineMS))
				select {
				case <-holdContext.Done():
					cancel()
					if q.Context().Err() == nil {
						ev.Failure = "hold_deadline"
						r.fail(errors.New("original target hold deadline"))
					}
					return
				case <-release:
					cancel()
					ev.ReleasedAtMS = time.Now().UnixMilli()
				}
				if q.Context().Err() != nil {
					return
				}
				require(mode == "loss" && ev.ReleasedAtMS < ev.HoldDeadlineMS, "only original live loss-attempt released")
			}
		} else {
			// A block-winning admission arriving before its second accepted receipt
			// cannot be used as this proof. Ordinary prospect-only requests pass through.
			r.mu.Lock()
			known := r.works[proof.JobID]
			r.mu.Unlock()
			require(known.Result.Work != nil, "unrelated admission has original captured job")
			prepared, pe := poolservice.PrepareDailyWork(r.expected.Source, r.expected.PublicKey, *known.Result.Work)
			check(pe, "unrelated original work")
			var header wire.BlockHeader
			check(header.Deserialize(bytes.NewReader(dec(proof.Header))), "original proof header")
			checked, pe := prepared.CheckNonce(q.Context(), header.Nonce)
			check(pe, "unrelated original nonce")
			require(checked.Header == proof.Header, "unrelated exact original header")
			if checked.Block {
				r.mu.Lock()
				first, ok := r.receipts[1]
				r.mu.Unlock()
				require(ok && proof.JobID == first.JobID && proof.Header == first.Header, "block admission requires its preceding accepted pool receipt")
			}

		}
	} else if q.URL.Path == "/pool/v3/issue" || q.URL.Path == "/pool/v3/issue-lookup" {
		var request poolservice.IssueRequest
		require(canonicalPublic(body, 1024, &request), "exact issue request")
		ev.RequestID = request.RequestID
	} else if q.URL.Path == "/pool/v3/submit" || q.URL.Path == "/pool/v3/lookup" {
		require(canonicalPublic(body, 1024, &poolRequest), "exact original pool request")
		ev.JobID = poolRequest.JobID
	}
	if q.Context().Err() != nil {
		return
	}
	up, e := http.NewRequestWithContext(q.Context(), q.Method, "https://"+backend+q.URL.EscapedPath(), bytes.NewReader(body))
	check(e, "original upstream request")
	up.Host = host
	up.Header = q.Header.Clone()
	up.Header.Del("Connection")
	up.Header.Del("Proxy-Authorization")
	ev.ForwardedAtMS = time.Now().UnixMilli()
	response, e := r.client.Do(up)
	if e != nil {
		ev.Failure = "upstream"
		http.Error(w, "upstream", 502)
		return
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, poolservice.MaxWorkResponseSize+1))
	ce := response.Body.Close()
	check(errors.Join(e, ce), "original response read/close")
	require(len(raw) <= poolservice.MaxWorkResponseSize, "response bound")
	ev.Status, ev.ResponseBytes, ev.ResponseSHA256 = response.StatusCode, len(raw), hx(digest(raw))
	if (q.URL.Path == "/pool/v3/issue" || q.URL.Path == "/pool/v3/issue-lookup") && response.StatusCode == 200 {
		var result poolservice.IssueResponse
		require(canonicalPublic(raw, poolservice.MaxWorkResponseSize, &result), "exact issued work frame")
		check(poolservice.ValidateRecoverableIssueResponse(r.expected, ev.RequestID, result, q.URL.Path == "/pool/v3/issue-lookup"), "original selected work response")
		if result.Result.Work != nil {
			r.locked(func() {
				if old, ok := r.works[result.Result.JobID]; ok {
					require(reflect.DeepEqual(old, result), "same original issued work replay")
				} else {
					require(len(r.works) < 8, "finite issued work inventory")
					r.works[result.Result.JobID] = result
				}
			})
		}
	}
	if (q.URL.Path == "/pool/v3/submit" || q.URL.Path == "/pool/v3/lookup") && response.StatusCode == 200 {
		var result poolservice.RecoverableSubmissionResponse
		require(canonicalPublic(raw, poolservice.MaxResponseSize, &result), "original pool receipt frame")
		sid, se := poolshare.DailyConfigID(r.expected.Source)
		check(se, "original source identity")
		pid, pe := registry.IssueProfileID(r.expected.IssueProfile)
		check(pe, "original profile identity")
		require(result.Version == 3 && result.BindingID == sid && result.SettlementConfigID == r.expected.SettlementConfigID && result.RecipientScript == r.expected.RecipientScript && result.IssueProfileID == pid, "original receipt envelope selection")
		receipt := result.Submission
		require(receipt.JobID == poolRequest.JobID && receipt.Nonce == poolRequest.Nonce, "receipt matches exact original pool request")
		r.mu.Lock()
		work := r.works[receipt.JobID]
		r.mu.Unlock()
		require(work.Result.Work != nil, "receipt original issued work captured")
		prepared, pe := poolservice.PrepareDailyWork(r.expected.Source, r.expected.PublicKey, *work.Result.Work)
		check(pe, "original receipt work")
		checked, pe := prepared.CheckNonce(q.Context(), receipt.Nonce)
		check(pe, "original receipt nonce")
		require(poolservice.SubmissionMatchesDaily(r.expected.Source, *work.Result.Work, receipt.Nonce, checked, receipt), "original exact receipt proof")
		if receipt.AcceptedSequence != 0 {
			require(receipt.AcceptedSequence <= 2 && receipt.Block && receipt.Pool, "exact two canonical reward prefix")
			r.locked(func() {
				if old, ok := r.receipts[receipt.AcceptedSequence]; ok {
					require(sourceReceipt(old) == sourceReceipt(receipt), "immutable accepted receipt")
				}
				r.receipts[receipt.AcceptedSequence] = receipt
			})
		}
	}
	drop := false
	notify := false
	if q.URL.Path == "/v3/admissions" {
		if response.StatusCode == 200 {
			var result prospectservice.ManifestAdmissionResult
			require(canonicalPublic(raw, prospectservice.MaxResponseSize, &result), "original admission response")
			require(result.Version == 3 && result.Record.Job.ID == proof.JobID && reflect.DeepEqual(result.Record.Proof, proof) && result.Record.Authorization == auth && result.Record.Birth.BornAtMS == result.Record.Job.IssuedAtMS, "same original admission identity")
			ev.Admission = &result
			r.locked(func() {
				r.admitted[admissionKey(proof.JobID, proof.Header)] = result
				if ev.Target {
					r.lastResponse = &result
					if ev.Mode == "loss" && !r.lost {
						require(result.Inserted && result.CurrentNodeChecked && result.ContextSource == "trusted-node-snapshot" && result.Record.Eligibility.Source == "canonical-winning-block", "actual new canonical target admission")
						r.lost = true
						r.mode = "hold-after-loss"
						r.lostResponse = &result
						r.release = make(chan struct{})
						r.released = false
						r.held = make(chan struct{})
						r.heldNotified = false
						drop = true
					} else if ev.Mode == "replay" {
						require(!result.Inserted && !result.CurrentNodeChecked && result.ContextSource == "historical-receipt" && r.lostResponse != nil && reflect.DeepEqual(result.Record, r.lostResponse.Record), "exact original historical receipt")
						notify = true
					} else {
						panic("unexpected target admission success mode")
					}
				}
			})
		} else if ev.Target {
			var result relayAPIError
			require(canonicalPublic(raw, 65536, &result) && result.Version == 3, "original target error response")
			ev.Error = &result
			r.mu.Lock()
			r.lastError = &result
			r.mu.Unlock()
			if ev.Mode == "expired" && response.StatusCode == 409 && result.Error.Code == "expired" && !result.Error.OutcomeUnknown {
				notify = true
			}
		}
	}
	if drop {
		h, ok := w.(http.Hijacker)
		require(ok, "original TLS hijacker")
		conn, _, e := h.Hijack()
		check(e, "withhold original success before headers")
		check(conn.Close(), "lost response connection joined")
		ev.Lost = true
		close(r.loss)
		return
	}
	for key, values := range response.Header {
		if key == "Connection" || key == "Transfer-Encoding" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	n, we := w.Write(raw)
	ev.DownstreamBytes = n
	if we != nil {
		ev.Failure = "downstream"
	}
	if notify && we == nil && n == len(raw) {
		r.mu.Lock()
		if !r.resultSent {
			r.resultSent = true
			close(r.result)
		}
		r.mu.Unlock()
	}
}

func (r *admissionTLSRelay) arm(mode string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	require(r.target != nil && r.targetActive == 0 && (mode == "loss" || mode == "replay" || mode == "expired"), "original joined target mode selection")
	r.mode = mode
	r.held = make(chan struct{})
	r.release = make(chan struct{})
	r.heldNotified = false
	r.released = false
	r.result = make(chan struct{})
	r.resultSent = false
	r.lastResponse = nil
	r.lastError = nil
}
func (r *admissionTLSRelay) releaseTarget() {
	r.mu.Lock()
	defer r.mu.Unlock()
	require(r.mode == "loss" && r.heldNotified && !r.released, "single original target release")
	r.released = true
	close(r.release)
}
func (r *admissionTLSRelay) targetJoined(ctx context.Context) {
	wait(ctx, 10*time.Second, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		check(r.failure, "original target relay")
		return r.targetActive == 0 && r.handlers == 0
	})
}
func (r *admissionTLSRelay) targetValue() admissionTarget {
	r.mu.Lock()
	defer r.mu.Unlock()
	require(r.target != nil, "original target exists")
	var v admissionTarget
	check(json.Unmarshal(encode(r.target), &v), "owned original target")
	return v
}
func (r *admissionTLSRelay) evidence() admissionRelayEvidence {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := admissionRelayEvidence{1, 512, r.target, r.mode, r.lost, r.lostResponse, append([]admissionRelayEvent{}, r.events...), r.failure != nil, r.closed, r.closed}
	var owned admissionRelayEvidence
	check(json.Unmarshal(encode(v), &owned), "owned bounded relay evidence")
	return owned
}
func (r *admissionTLSRelay) close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.closeAttempted {
		e := r.closeErr
		r.mu.Unlock()
		return e
	}
	r.closeAttempted = true
	r.closing = true
	r.mu.Unlock()
	e := r.server.Close()
	se := <-r.done
	r.active.Wait()
	r.client.CloseIdleConnections()
	if errors.Is(se, http.ErrServerClosed) {
		se = nil
	}
	r.mu.Lock()
	r.closed = true
	r.closeErr = errors.Join(e, se, r.failure)
	e = r.closeErr
	r.mu.Unlock()
	return e
}
