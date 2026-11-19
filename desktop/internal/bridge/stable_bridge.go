// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
)

// StableBridge runs both closed command families on one owned helper pipe.
// The pending command, never a response-controlled discriminator, selects the
// decoder. A stable failure stays a typed code across the webview boundary.
type StableBridge struct {
	payoutBinding                *pilot.PoolPayoutBinding
	payoutAttempt, payoutCapture *payoutExpectation
	payoutRows                   *pilot.PoolPayoutPage
	evidence                     [2]*evidenceExpectation
	evidenceSerial               uint64
	poolStartupComplete          bool
	poolHandshakeUsed            bool
	poolHandshakePending         bool
	poolMode                     bool
	poolSourceID                 string
	poolExpectedDescriptor       string
	poolSourcePin                *registry.ManifestAuthorityPin
	careersEnabled               bool
	careerActivation             *careerstate.Activation
	careerSelection              *careerPageExpectation
	// History state is also invalidated by the reader failure path under mu.
	history                   *careerHistoryCorrelation
	historyFatal              bool
	cmd                       *exec.Cmd
	input                     io.WriteCloser
	responses                 chan stableFrame
	failed, done, readDone    chan struct{}
	gate                      chan struct{}
	once                      sync.Once
	mu                        sync.Mutex
	failure, waitErr          error
	nextID                    uint64
	expected                  *stableExpected
	view                      *pilot.ManifestViewPin
	manifestView              *pilot.ManifestViewPin
	storeID                   string
	stablePin                 *gymstable.Pin
	careerPreviewReady        bool
	careerPreviewJobs         map[string]careerPreviewJobExpectation
	careerPreviewSnapshotJobs map[string]bool
	previewSelection          *previewSelection
	previewReady              bool
	previewJobs               map[string]previewJobExpectation
	previewSnapshotJobs       map[string]bool
	entriesEnabled            bool
	eventsEnabled             bool
	boutsEnabled              bool
	resultsEnabled            bool
	resultPin                 *resultAuthority
	result                    *resultCorrelation
	boutPin                   *boutAuthority
	bout                      *boutCorrelation
	boutPresentation          *boutPresentation
	cardsEnabled              bool
	cardPin                   *cardAuthority
	card                      *cardCorrelation
	seedsEnabled              bool
	seedPin                   *seedAuthority
	seed                      *seedCorrelation
	// The immutable coordinator pin survives logout; event correlation does not.
	eventPin *eventAuthority
	event    *eventCorrelation
	// Entry correlation is also cleared by the reader's failure path. All
	// access to this pointer and its contents is protected by mu.
	entry *entryCorrelation
	// presentation is also cleared by the reader's failure path, under mu.
	presentation *previewPresentation
	// At most one active, one queued and one retained completed explicit job.
	// Access is serialized by gate; entries contain public request metadata only.
	jobs         map[string]stableJobExpectation
	snapshotJobs map[string]bool
}
type stableJobExpectation struct {
	request  pilot.StableRequest
	terminal bool
}
type stableExpected struct {
	payout        bool
	evidence      bool
	pool          bool
	history       bool
	careerPreview bool
	career        bool
	id            uint64
	stable        bool
	preview       bool
	entry         bool
	event         bool
	seed          bool
	bout          bool
	result        bool
	card          bool
}
type stableFrame struct {
	payout        *pilot.PoolPayoutResponse
	evidence      *pilot.FighterEvidenceResponse
	pool          *pilot.PoolResponse
	history       *pilot.CareerHistoryResponse
	careerPreview *pilot.CareerPreviewResponse
	career        *pilot.CareerResponse
	manifest      *pilot.ManifestResponse
	stable        *pilot.StableResponse
	preview       *pilot.PreviewResponse
	entry         *pilot.EntryResponse
	event         *pilot.EventResponse
	seed          *pilot.SeedResponse
	bout          *pilot.BoutResponse
	result        *pilot.ResultResponse
	card          *pilot.CardResponse
}

func LaunchStable(helper, dataDir, node string) (*StableBridge, error) {
	return startStable(exec.Command(helper, "--candidate-manifests", "--candidate-stable", "--data-dir", dataDir, "--node-binary", node))
}
func startStable(cmd *exec.Cmd) (*StableBridge, error) {
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, ErrUnavailable
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, ErrUnavailable
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, ErrUnavailable
	}
	b := &StableBridge{cmd: cmd, input: input, responses: make(chan stableFrame, 1), failed: make(chan struct{}), done: make(chan struct{}), readDone: make(chan struct{}), gate: make(chan struct{}, 1)}
	go b.read(output)
	go func() {
		err := cmd.Wait()
		b.mu.Lock()
		b.waitErr = err
		b.mu.Unlock()
		b.fail(ErrUnavailable)
		close(b.done)
	}()
	return b, nil
}
func (b *StableBridge) fail(err error) {
	b.once.Do(func() {
		b.mu.Lock()
		b.failure = err
		b.payoutBinding, b.payoutAttempt, b.payoutCapture, b.payoutRows = nil, nil, nil, nil
		b.careerSelection = nil
		b.history = nil
		b.historyFatal = true
		b.presentation = nil
		b.previewSelection = nil
		b.entry = nil
		b.event = nil
		b.seed = nil
		b.card = nil
		b.bout = nil
		b.result = nil
		b.boutPresentation = nil
		b.mu.Unlock()
		close(b.failed)
		_ = b.input.Close()
	})
}
func (b *StableBridge) read(r io.Reader) {
	defer close(b.readDone)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), pilot.MaxResponseSize)
	for scanner.Scan() {
		b.mu.Lock()
		expected := b.expected
		b.mu.Unlock()
		if expected == nil {
			b.fail(ErrUnavailable)
			return
		}
		var frame stableFrame
		if expected.payout {
			value, err := pilot.ParsePoolPayoutResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.payout = &value
		} else if expected.evidence {
			value, err := pilot.ParseFighterEvidenceResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.evidence = &value
		} else if expected.pool {
			value, err := pilot.ParsePoolResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.pool = &value
		} else if expected.history {
			value, err := pilot.ParseCareerHistoryResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.history = &value
		} else if expected.careerPreview {
			value, err := pilot.ParseCareerPreviewResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.careerPreview = &value
		} else if expected.career {
			value, err := pilot.ParseCareerResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.career = &value
		} else if expected.result {
			value, err := pilot.ParseResultResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.result = &value
		} else if expected.bout {
			value, err := pilot.ParseBoutResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.bout = &value
		} else if expected.card {
			value, err := pilot.ParseCardResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.card = &value
		} else if expected.seed {
			value, err := pilot.ParseSeedResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.seed = &value
		} else if expected.event {
			value, err := pilot.ParseEventResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.event = &value
		} else if expected.entry {
			value, err := pilot.ParseEntryResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.entry = &value
		} else if expected.preview {
			value, err := pilot.ParsePreviewResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.preview = &value
		} else if expected.stable {
			value, err := pilot.ParseStableResponse(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.stable = &value
		} else {
			value, err := decodeManifest(scanner.Bytes())
			if err != nil || value.ID != expected.id {
				b.fail(ErrUnavailable)
				return
			}
			frame.manifest = &value
		}
		b.mu.Lock()
		b.expected = nil
		b.mu.Unlock()
		select {
		case b.responses <- frame:
		case <-b.failed:
			return
		default:
			b.fail(ErrUnavailable)
			return
		}
	}
	b.fail(ErrUnavailable)
}

// begin/finish cover pipe dispatch and identity cache updates as one serialized
// operation. No hashing or store access occurs in the native process.
func (b *StableBridge) begin(ctx context.Context) (uint64, error) {
	select {
	case b.gate <- struct{}{}:
	default:
		return 0, ErrBusy
	}
	select {
	case <-b.failed:
		<-b.gate
		return 0, ErrUnavailable
	default:
	}
	if ctx.Err() != nil {
		<-b.gate
		return 0, ctx.Err()
	}
	b.nextID++
	if b.nextID > manifestSafeInteger {
		b.fail(ErrUnavailable)
		<-b.gate
		return 0, ErrUnavailable
	}
	return b.nextID, nil
}
func (b *StableBridge) exchange(ctx context.Context, id uint64, stable bool, raw []byte) (stableFrame, error) {
	return b.exchangeFamily(ctx, id, stable, false, raw)
}
func (b *StableBridge) exchangeFamily(ctx context.Context, id uint64, stable, preview bool, raw []byte) (stableFrame, error) {
	return b.exchangeEntryFamily(ctx, id, stable, preview, false, raw)
}
func (b *StableBridge) exchangeEntryFamily(ctx context.Context, id uint64, stable, preview, entry bool, raw []byte) (stableFrame, error) {
	return b.exchangeEventFamily(ctx, id, stable, preview, entry, false, raw)
}
func (b *StableBridge) exchangeEventFamily(ctx context.Context, id uint64, stable, preview, entry, event bool, raw []byte) (stableFrame, error) {
	return b.exchangeSeedFamily(ctx, id, stable, preview, entry, event, false, raw)
}
func (b *StableBridge) exchangeSeedFamily(ctx context.Context, id uint64, stable, preview, entry, event, seed bool, raw []byte) (stableFrame, error) {
	return b.exchangeCardFamily(ctx, id, stable, preview, entry, event, seed, false, raw)
}
func (b *StableBridge) exchangeCardFamily(ctx context.Context, id uint64, stable, preview, entry, event, seed, card bool, raw []byte) (stableFrame, error) {
	return b.exchangeBoutFamily(ctx, id, stable, preview, entry, event, seed, card, false, raw)
}
func (b *StableBridge) exchangeBoutFamily(ctx context.Context, id uint64, stable, preview, entry, event, seed, card, bout bool, raw []byte) (stableFrame, error) {
	return b.exchangeResultFamily(ctx, id, stable, preview, entry, event, seed, card, bout, false, raw)
}
func (b *StableBridge) exchangeResultFamily(ctx context.Context, id uint64, stable, preview, entry, event, seed, card, bout, result bool, raw []byte) (stableFrame, error) {
	return b.exchangeCareerFamily(ctx, id, stable, preview, entry, event, seed, card, bout, result, false, raw)
}
func (b *StableBridge) exchangeCareerFamily(ctx context.Context, id uint64, stable, preview, entry, event, seed, card, bout, result, career bool, raw []byte) (stableFrame, error) {
	return b.exchangeCareerPreviewFamily(ctx, id, stable, preview, entry, event, seed, card, bout, result, career, false, raw)
}
func (b *StableBridge) exchangeCareerPreviewFamily(ctx context.Context, id uint64, stable, preview, entry, event, seed, card, bout, result, career, careerPreview bool, raw []byte) (stableFrame, error) {
	return b.exchangeHistoryFamily(ctx, id, stable, preview, entry, event, seed, card, bout, result, career, careerPreview, false, raw)
}
func (b *StableBridge) exchangeHistoryFamily(ctx context.Context, id uint64, stable, preview, entry, event, seed, card, bout, result, career, careerPreview, history bool, raw []byte) (stableFrame, error) {
	return b.exchangeExpected(ctx, &stableExpected{history: history, careerPreview: careerPreview, career: career, id: id, stable: stable, preview: preview, entry: entry, event: event, seed: seed, card: card, bout: bout, result: result}, raw)
}
func (b *StableBridge) exchangeExpected(ctx context.Context, expected *stableExpected, raw []byte) (stableFrame, error) {
	b.mu.Lock()
	b.expected = expected
	b.mu.Unlock()
	raw = append(raw, '\n')
	if d, ok := b.input.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	if n, err := b.input.Write(raw); err != nil || n != len(raw) {
		b.fail(ErrUnavailable)
		return stableFrame{}, ErrUnavailable
	}
	select {
	case f := <-b.responses:
		return f, nil
	case <-b.failed:
		// A helper may exit just after publishing a complete final response.
		select {
		case f := <-b.responses:
			return f, nil
		default:
			return stableFrame{}, ErrUnavailable
		}
	case <-ctx.Done():
		b.fail(ErrUnavailable)
		return stableFrame{}, ErrUnavailable
	}
}
func (b *StableBridge) Close() error {
	b.fail(ErrClosed)
	<-b.done
	<-b.readDone
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.waitErr != nil {
		return ErrUnavailable
	}
	return nil
}

func (b *StableBridge) Request(ctx context.Context, command string, sequence uint64) (pilot.ManifestResponse, error) {
	switch command {
	case "snapshot", "start", "stop", "logout", "reset_fixture", "refresh_manifest_prospects":
		if sequence != 0 {
			return pilot.ManifestResponse{}, ErrUnavailable
		}
	case "manifest_portrait", "manifest_receipt":
		if sequence == 0 || sequence > pilot.MaxManifestSequence {
			return pilot.ManifestResponse{}, ErrUnavailable
		}
	default:
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return b.manifestRequest(ctx, pilot.ManifestRequest{Command: command, Sequence: sequence})
}
func (b *StableBridge) RequestFixture(ctx context.Context, fixture string) (pilot.ManifestResponse, error) {
	if fixture != "zero256" && fixture != "incrementing256" {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return b.manifestRequest(ctx, pilot.ManifestRequest{Command: "login_fixture", Fixture: fixture})
}
func (b *StableBridge) RequestPath(ctx context.Context, command, path string) (pilot.ManifestResponse, error) {
	if !oneOf(command, "export_recovery", "verify_backup") || !pilot.ValidRecoveryPath(path) {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return b.manifestRequest(ctx, pilot.ManifestRequest{Command: command, Path: path})
}
func (b *StableBridge) ExportManifest(ctx context.Context, sequence uint64, viewID, prospectID, path string) (pilot.ManifestResponse, error) {
	if sequence == 0 || sequence > pilot.MaxManifestSequence || !publicHex.MatchString(viewID) || !publicHex.MatchString(prospectID) || !pilot.ValidRecoveryPath(path) {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return b.manifestRequest(ctx, pilot.ManifestRequest{Command: "export_manifest_receipt", Sequence: sequence, ViewID: viewID, ProspectID: prospectID, Path: path})
}
func (b *StableBridge) ReleaseManifest(ctx context.Context, viewID, prospectID string) (pilot.ManifestResponse, error) {
	if !publicHex.MatchString(viewID) || !publicHex.MatchString(prospectID) {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	return b.manifestRequest(ctx, pilot.ManifestRequest{Command: "release_manifest_receipt", ViewID: viewID, ProspectID: prospectID})
}
func (b *StableBridge) manifestRequest(ctx context.Context, req pilot.ManifestRequest) (pilot.ManifestResponse, error) {
	if b.poolMode {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	id, err := b.begin(ctx)
	if err != nil {
		return pilot.ManifestResponse{}, err
	}
	defer func() { <-b.gate }()
	req.ID = id
	raw, err := json.Marshal(req)
	if err != nil || len(raw) > pilot.MaxRequestSize {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	// Controls which change identity invalidate the cache even if their ACK is
	// lost. A fresh stable_snapshot must establish the next complete identity.
	if oneOf(req.Command, "login_fixture", "logout", "reset_fixture") {
		b.clearStableView()
		b.manifestView = nil
	}
	if req.Command == "stop" {
		// Stop replaces the concrete Stable owner. A lost ACK cannot leave a
		// pageable handle claiming that the previous owner is still live.
		b.clearCareerHistory(false)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	frame, err := b.exchange(ctx, id, false, raw)
	if err != nil || frame.manifest == nil {
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	response := *frame.manifest
	if !response.OK {
		return pilot.ManifestResponse{}, errors.New("The local candidate pilot could not complete this request.")
	}
	correct := false
	switch req.Command {
	case "manifest_portrait":
		correct = response.Portrait != nil && response.Portrait.Sequence == req.Sequence
	case "manifest_receipt":
		correct = response.ManifestReceipt != nil && response.ManifestReceipt.Record.Sequence == req.Sequence
	default:
		correct = response.Snapshot != nil
	}
	if !correct {
		b.fail(ErrUnavailable)
		return pilot.ManifestResponse{}, ErrUnavailable
	}
	if response.Snapshot != nil && (response.Snapshot.View == nil || b.view != nil && *b.view != *response.Snapshot.View) {
		b.clearStableView()
	}
	if response.Snapshot != nil {
		b.manifestView = nil
		if response.Snapshot.View != nil {
			v := *response.Snapshot.View
			b.manifestView = &v
		}
	}
	return response, nil
}

// StableRequest returns a closed result even for local preflight failures. ID1
// on a failure before dispatch is only a local envelope ID, not a journal ID.
// Apply's durable operation ID stays with its caller across an unknown result.
func (b *StableBridge) StableRequest(ctx context.Context, req pilot.StableRequest) pilot.StableResponse {
	fail := func(id uint64, code string) pilot.StableResponse {
		if id == 0 {
			id = 1
		}
		return pilot.StableResponse{ID: id, Error: code}
	}
	id, err := b.begin(ctx)
	if err != nil {
		code := "stable_unavailable"
		if errors.Is(err, ErrBusy) {
			code = "busy"
		}
		return fail(0, code)
	}
	defer func() { <-b.gate }()
	req.ID = id
	raw, err := pilot.MarshalStableRequest(req)
	if err != nil {
		return fail(id, "invalid_request")
	}
	if req.Command != "stable_snapshot" && (b.view == nil || b.view.ViewID != req.ViewID || b.storeID == "") {
		return fail(id, "stale_view")
	}
	// Do not dispatch a new action if its ACK could exceed the correlation
	// bound. A fresh authoritative snapshot can reconcile ambiguous live IDs.
	if stableStartsJob(req.Command) && !b.canTrackStableJob() {
		return fail(id, "busy")
	}
	unknown := stableMutation(req.Command)
	if oneOf(req.Command, "stable_job", "stable_cancel") {
		if expected, ok := b.jobs[req.JobID]; ok {
			unknown = stableMutation(expected.request.Command)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	frame, err := b.exchange(ctx, id, true, raw)
	if err != nil || frame.stable == nil {
		if unknown {
			return fail(id, "outcome_unknown")
		}
		return fail(id, "stable_unavailable")
	}
	response := *frame.stable
	if !response.OK {
		if oneOf(response.Error, "unauthorized", "stale_view") {
			b.clearStableView()
		}
		if oneOf(response.Error, "stable_unavailable", "stable_corrupt", "stable_clock", "stable_observation_unknown") {
			b.clearCareerHistory(true)
		}
		if unknown && oneOf(response.Error, "unauthorized", "stale_view", "stable_unavailable", "stable_not_found") {
			return fail(id, "outcome_unknown")
		}
		return response
	}
	if b.poolMode && response.Snapshot != nil && response.Snapshot.Status != nil && response.Snapshot.Status.Pin.Version != gymstable.SharedPinVersion2 || !b.correlateStable(req, response) {
		b.fail(ErrUnavailable)
		if unknown {
			return fail(id, "outcome_unknown")
		}
		return fail(id, "stable_unavailable")
	}
	if response.Snapshot != nil {
		b.rememberStableSnapshot(*response.Snapshot)
	}
	if response.Job != nil && !b.trackStableJob(req, *response.Job) {
		b.fail(ErrUnavailable)
		if unknown {
			return fail(id, "outcome_unknown")
		}
		return fail(id, "stable_unavailable")
	}
	if response.Job != nil && !b.rememberCareerStableJob(req, *response.Job) {
		b.fail(ErrUnavailable)
		return fail(id, "stable_unavailable")
	}
	return response
}

func stableStartsJob(command string) bool {
	return oneOf(command, "stable_sync", "stable_recent", "stable_inventory", "stable_roster", "stable_prepare_slot", "stable_apply_slot")
}
func stableMutation(command string) bool {
	return oneOf(command, "stable_prepare_slot", "stable_apply_slot", "stable_abandon_operation")
}
func stableTerminal(state string) bool { return state != "queued" && state != "running" }
func (b *StableBridge) clearStableView() {
	b.mu.Lock()
	b.evidence = [2]*evidenceExpectation{}
	b.payoutBinding, b.payoutAttempt, b.payoutCapture, b.payoutRows = nil, nil, nil, nil
	b.careerSelection = nil
	b.history = nil
	b.historyFatal = false
	b.mu.Unlock()
	b.view = nil
	b.storeID = ""
	b.stablePin = nil
	b.jobs = nil
	b.snapshotJobs = nil
	b.clearPreviewView()
	b.clearCareerPreviewView()
	b.mu.Lock()
	b.previewSelection = nil
	b.mu.Unlock()
	b.clearEntryView()
	b.clearEventView()
	b.clearSeedView()
	b.clearCardView()
	b.clearBoutView()
	b.clearResultView()
}
func (b *StableBridge) rememberStableSnapshot(s pilot.StableSnapshot) {
	if s.View == nil {
		b.clearStableView()
		return
	}
	if b.view == nil || *b.view != *s.View || b.storeID != s.Status.StoreID || b.stablePin != nil && *b.stablePin != s.Status.Pin {
		b.clearStableView()
	}
	v := *s.View
	b.view = &v
	b.storeID = s.Status.StoreID
	p := s.Status.Pin
	b.stablePin = &p
	b.snapshotJobs = map[string]bool{}
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			b.snapshotJobs[id] = true
		}
	}
	for id := range b.jobs {
		if !b.snapshotJobs[id] {
			delete(b.jobs, id)
		}
	}
}
func (b *StableBridge) canTrackStableJob() bool {
	if len(b.jobs) < 3 {
		return true
	}
	for _, e := range b.jobs {
		if e.terminal {
			return true
		}
	}
	return false
}
func (b *StableBridge) trackStableJob(req pilot.StableRequest, j pilot.StableJob) bool {
	expected, known := b.jobs[j.JobID]
	if stableStartsJob(req.Command) {
		req.ID = 0
		if known && expected.request != req {
			return false
		}
		expected.request = req
		known = true
	} else if !known {
		// Automatic catch-up jobs have no explicit request tuple. They are
		// discoverable only through a current authoritative snapshot and may
		// never turn into a page or mutation under that same job identity.
		return b.snapshotJobs[j.JobID] && j.Kind == "sync"
	}
	if b.jobs == nil {
		b.jobs = map[string]stableJobExpectation{}
	}
	if stableTerminal(j.State) {
		// The helper retains only one completed result. Removing a known
		// terminal entry cannot discard correlation for a still active job.
		for id, e := range b.jobs {
			if id != j.JobID && e.terminal {
				delete(b.jobs, id)
			}
		}
		expected.terminal = true
	} else if expected.terminal {
		return false
	}
	if _, exists := b.jobs[j.JobID]; !exists && len(b.jobs) >= 3 {
		for id, e := range b.jobs {
			if e.terminal {
				delete(b.jobs, id)
				break
			}
		}
	}
	if _, exists := b.jobs[j.JobID]; !exists && len(b.jobs) >= 3 {
		return false
	}
	b.jobs[j.JobID] = expected
	return true
}
func (b *StableBridge) correlateStable(req pilot.StableRequest, response pilot.StableResponse) bool {
	if req.Command == "stable_snapshot" {
		s := response.Snapshot
		if s == nil {
			return false
		}
		if s.View == nil {
			return s.Status == nil
		}
		if s.Status == nil || !validManifestView(*s.View) {
			return false
		}
		p := s.Status.Pin
		id, err := gymstable.ID(p)
		return err == nil && id == s.Status.StoreID && p.AccountAuthorityID == s.View.AccountAuthorityID && p.AccountID == s.View.AccountID && p.Source == s.View.AuthorityPin
	}
	if b.view == nil {
		return false
	}
	if oneOf(req.Command, "stable_operation", "stable_pending", "stable_abandon_operation") {
		o := response.Operation
		if o == nil || o.View != *b.view || o.StoreID != b.storeID {
			return false
		}
		if o.Operation == nil {
			return req.Command == "stable_pending"
		}
		if req.Command != "stable_pending" && o.Operation.Intent.OperationID != req.OperationID {
			return false
		}
		return true
	}
	j := response.Job
	if j == nil || j.View != *b.view || j.StoreID != b.storeID {
		return false
	}
	if oneOf(req.Command, "stable_job", "stable_cancel") {
		if j.JobID != req.JobID {
			return false
		}
		if expected, ok := b.jobs[j.JobID]; ok {
			if expected.terminal && !stableTerminal(j.State) {
				return false
			}
			return b.correlateStableJob(expected.request, *j)
		}
		return b.snapshotJobs[j.JobID] && j.Kind == "sync"
	}
	return b.correlateStableJob(req, *j)
}
func (b *StableBridge) correlateStableJob(req pilot.StableRequest, j pilot.StableJob) bool {
	if b.view == nil || j.View != *b.view || j.StoreID != b.storeID {
		return false
	}
	switch req.Command {
	case "stable_sync":
		return j.Kind == "sync"
	case "stable_recent":
		return j.Kind == "inventory" && (j.Page == nil || j.Page.After == max(j.Page.Status.Checkpoint, uint64(8))-8 && j.Page.NextAfter == j.Page.Status.Checkpoint && len(j.Page.Items) <= 8)
	case "stable_inventory":
		return j.Kind == "inventory" && (j.Page == nil || j.Page.After == req.After && len(j.Page.Items) <= int(req.Limit))
	case "stable_roster":
		end := min(req.FirstSlot+req.Limit, uint32(11))
		if end == 11 {
			end = 0
		}
		return j.Kind == "roster" && (j.Roster == nil || j.Roster.FirstSlot == req.FirstSlot && j.Roster.NextSlot == end && len(j.Roster.Items) <= int(req.Limit))
	case "stable_prepare_slot":
		return j.Kind == "prepare" && (j.Intent == nil || j.Intent.Request == (gymstable.SlotRequest{ExpectedRevision: req.ExpectedRevision, Slot: req.Slot, ProspectID: req.ProspectID}) && j.Intent.ViewID == req.ViewID && j.Intent.Generation == b.view.Generation)
	case "stable_apply_slot":
		return j.Kind == "apply" && j.OperationID == req.OperationID && (j.Mutation == nil || j.Mutation.Receipt.OperationID == req.OperationID && j.Mutation.Receipt.ViewID == req.ViewID && j.Mutation.Receipt.Generation == b.view.Generation)
	}
	return false
}
