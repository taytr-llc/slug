// Copyright (c) 2026 The SLUG developers. ISC license.
// slugrestore is the separate explicit Phase-A file-restored gym consumer.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
)

const usage = `slugrestore COMMAND --directory PATH --public-config FILE --node-config FILE [options]
Commands: init, resume, status, fighters, discover, transfer, recover,
          resume-action, deliver, observe, abandon, actions, request-id
init/resume require --recovery-file FILE until READY. No phrase or password
literal is accepted. The node must use the existing literal-loopback HTTP profile.
transfer: --prospect-id ID --recipient-gym KEY [--request-id ID]
recover: --commit-id TXID --kind recover_reveal|recover_refund [--request-id ID]
resume-action: --request-id ID [--prospect-id ID for a new transfer]
deliver/observe: --request-id ID --leg commit|spend
status/abandon: --request-id ID for an exact saved action
Signing and delivery each require typing the displayed one-use approval.
No command mines, replaces a hosted wallet, or automatically resends transactions.
Output is bounded public JSON lines; errors use closed codes, never source strings.
`

var errArguments = errors.New("invalid arguments")

type options struct {
	command                                         string
	config                                          pilot.RestoredGymConfig
	request, prospect, recipient, commit, kind, leg string
}

func parse(args []string) (options, error) {
	var o options
	if len(args) == 0 {
		return o, flag.ErrHelp
	}
	o.command = args[0]
	if o.command == "help" || o.command == "--help" || o.command == "-h" {
		return o, flag.ErrHelp
	}
	fs := flag.NewFlagSet("slugrestore", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.config.Directory, "directory", "", "")
	fs.StringVar(&o.config.RecoveryFile, "recovery-file", "", "")
	fs.StringVar(&o.config.PublicConfigFile, "public-config", "", "")
	fs.StringVar(&o.config.NodeConfigFile, "node-config", "", "")
	fs.StringVar(&o.request, "request-id", "", "")
	fs.StringVar(&o.prospect, "prospect-id", "", "")
	fs.StringVar(&o.recipient, "recipient-gym", "", "")
	fs.StringVar(&o.commit, "commit-id", "", "")
	fs.StringVar(&o.kind, "kind", "", "")
	fs.StringVar(&o.leg, "leg", "", "")
	if e := fs.Parse(args[1:]); e != nil {
		return o, errArguments
	}
	if fs.NArg() != 0 {
		return o, errArguments
	}
	switch o.command {
	case "init", "resume", "status", "fighters", "discover", "transfer", "recover", "resume-action", "deliver", "observe", "abandon", "actions", "request-id":
	default:
		return o, errArguments
	}
	if e := validateOptions(o, fs); e != nil {
		return o, e
	}
	return o, nil
}
func publicID(v string) bool {
	if len(v) != 64 || strings.ToLower(v) != v || v == strings.Repeat("0", 64) {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil
}
func gymKey(v string) bool {
	if len(v) != 66 || strings.ToLower(v) != v || (v[:2] != "02" && v[:2] != "03") {
		return false
	}
	b, e := hex.DecodeString(v)
	if e != nil {
		return false
	}
	_, e = btcec.ParsePubKey(b)
	return e == nil
}
func validateOptions(o options, fs *flag.FlagSet) error {
	allowed := map[string]bool{}
	if o.command != "request-id" {
		for _, name := range []string{"directory", "public-config", "node-config", "recovery-file"} {
			allowed[name] = true
		}
		if o.config.Directory == "" || o.config.PublicConfigFile == "" || o.config.NodeConfigFile == "" {
			return errArguments
		}
	}
	switch o.command {
	case "init":
		if o.config.RecoveryFile == "" {
			return errArguments
		}
	case "status":
		allowed["request-id"] = true
	case "transfer":
		allowed["request-id"], allowed["prospect-id"], allowed["recipient-gym"] = true, true, true
		if !publicID(o.prospect) || !gymKey(o.recipient) {
			return errArguments
		}
	case "recover":
		allowed["request-id"], allowed["commit-id"], allowed["kind"] = true, true, true
		if !publicID(o.commit) || (o.kind != "recover_reveal" && o.kind != "recover_refund") {
			return errArguments
		}
	case "resume-action":
		allowed["request-id"], allowed["prospect-id"] = true, true
		if !publicID(o.request) {
			return errArguments
		}
	case "deliver", "observe":
		allowed["request-id"], allowed["leg"] = true, true
		if !publicID(o.request) || (o.leg != "commit" && o.leg != "spend") {
			return errArguments
		}
	case "abandon":
		allowed["request-id"] = true
		if !publicID(o.request) {
			return errArguments
		}
	}
	if o.request != "" && !publicID(o.request) || o.prospect != "" && !publicID(o.prospect) {
		return errArguments
	}
	invalid := false
	fs.Visit(func(f *flag.Flag) {
		if !allowed[f.Name] {
			invalid = true
		}
	})
	if invalid {
		return errArguments
	}
	return nil
}

// Main owns the concrete output descriptors for this process. Cancellation
// interrupts a blocked pipe write and joins both first-close attempts before
// shutdown; no background output writer or OS-close retry is retained.
func interruptOutputs(ctx context.Context, files ...*os.File) func() error {
	done := make(chan struct{})
	var result error
	stop := context.AfterFunc(ctx, func() {
		for _, f := range files {
			if f != nil {
				result = errors.Join(result, f.Close())
			}
		}
		close(done)
	})
	return func() error {
		if !stop() {
			<-done
			return result
		}
		return nil
	}
}

func writeJSON(w io.Writer, v any) error {
	raw, e := json.Marshal(v)
	if e != nil || len(raw) > 65536 {
		return fightercarrier.ErrCapacity
	}
	raw = append(raw, '\n')
	n, e := w.Write(raw)
	if e == nil && n != len(raw) {
		e = io.ErrShortWrite
	}
	return e
}
func newRequestID() (string, error) {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return hex.EncodeToString(b[:]), nil
}

// The original stdin descriptor is closed only on cancellation/deadline, then
// the cancellation callback is joined. There is no detached input goroutine.
func confirm(ctx context.Context, input *os.File, reader *bufio.Reader, output io.Writer, review string, expires int64) error {
	ctx, cancel := context.WithDeadline(ctx, time.UnixMilli(expires))
	defer cancel()
	if _, e := fmt.Fprintln(output, "Type approve "+review+" to approve this exact review:"); e != nil {
		return e
	}
	joined := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = input.Close(); close(joined) })
	line, e := reader.ReadSlice('\n')
	if !stop() {
		<-joined
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if e != nil || len(line) > 128 || strings.TrimSuffix(string(line), "\n") != "approve "+review {
		return fightercarrier.ErrUnauthorized
	}
	return nil
}
func selectedID(o *options, output io.Writer) error {
	if o.request == "" {
		id, e := newRequestID()
		if e != nil {
			return e
		}
		o.request = id
	}
	// Disclose the public correlation before any durable action, so interrupted
	// preparation is recovered only by this exact request (or explicit list).
	return writeJSON(output, struct {
		RequestID string `json:"request_id"`
	}{o.request})
}
func recoveryReview(ctx context.Context, s *pilot.RestoredGymSession, o options) (pilot.RestoredTransferReview, error) {
	p, e := s.BeginRecoveryReview(ctx, o.request, o.commit, o.kind)
	if e != nil {
		return pilot.RestoredTransferReview{}, e
	}
	// Eight original branches are capped at4096; native84 matching examines only
	// two. Each library step is capped at128 raw children; keep a finite bound.
	for step := 0; step < 65; step++ {
		p, e = s.RecoveryReviewStep(ctx)
		if e != nil {
			return pilot.RestoredTransferReview{}, e
		}
		if p.Complete {
			if !p.Matched || p.Review == nil {
				return pilot.RestoredTransferReview{}, fightercarrier.ErrNotFound
			}
			return *p.Review, nil
		}
	}
	return pilot.RestoredTransferReview{}, fightercarrier.ErrCapacity
}

// A single CLI invocation retains one exact replay through bounded slices.
// Errors are terminal; no timeout/stale retry creates replacement authority.
type refreshSession interface {
	BeginRefresh(context.Context) (fighterownership.Progress, error)
	RefreshStep(context.Context, uint32) (fighterownership.Progress, error)
	CancelRefresh(context.Context) error
}

func refreshHead(ctx context.Context, s refreshSession) (head fighterownership.HeadRef, resultErr error) {
	p, e := s.BeginRefresh(ctx)
	if e != nil {
		return fighterownership.HeadRef{}, e
	}
	defer func() { resultErr = errors.Join(resultErr, s.CancelRefresh(ctx)) }()
	for !p.Ready {
		if e = ctx.Err(); e != nil {
			return fighterownership.HeadRef{}, e
		}
		p, e = s.RefreshStep(ctx, 8)
		if e != nil {
			return fighterownership.HeadRef{}, e
		}
	}
	if p.Head == nil || p.Cutoff != nil {
		return fighterownership.HeadRef{}, fightercarrier.ErrCorrupt
	}
	return *p.Head, nil
}

func run(ctx context.Context, args []string, input *os.File, output, prompt io.Writer) (resultErr error) {
	o, e := parse(args)
	if errors.Is(e, flag.ErrHelp) {
		_, e = io.WriteString(output, usage)
		return e
	}
	if e != nil {
		return e
	}
	if o.command == "request-id" {
		id, e := newRequestID()
		if e != nil {
			return e
		}
		return writeJSON(output, struct {
			RequestID string `json:"request_id"`
		}{id})
	}
	var s *pilot.RestoredGymSession
	if o.command == "init" {
		s, e = pilot.CreateRestoredGymSession(ctx, o.config)
	} else {
		s, e = pilot.OpenRestoredGymSession(ctx, o.config)
	}
	if e != nil {
		return e
	}
	defer func() { resultErr = errors.Join(resultErr, s.Close()) }()
	reader := bufio.NewReaderSize(input, 256)
	switch o.command {
	case "init", "resume":
		v, e := s.Restore(ctx)
		if e != nil {
			return e
		}
		return writeJSON(output, v)
	case "status":
		if o.request != "" {
			v, e := s.Action(ctx, o.request)
			if e != nil {
				return e
			}
			return writeJSON(output, v)
		}
		v, e := s.Status(ctx)
		if e != nil {
			return e
		}
		return writeJSON(output, v)
	case "actions":
		for after := uint32(0); after < 128; {
			v, e := s.Actions(ctx, after)
			if e != nil {
				return e
			}
			if e = writeJSON(output, v); e != nil {
				return e
			}
			if v.Complete {
				return nil
			}
			if v.Next <= after {
				return fightercarrier.ErrCorrupt
			}
			after = v.Next
		}
		return fightercarrier.ErrCapacity
	case "observe":
		v, e := s.ObserveAction(ctx, o.request, o.leg)
		if e != nil {
			return e
		}
		return writeJSON(output, v)
	case "abandon":
		v, e := s.AbandonAction(ctx, o.request)
		if e != nil {
			return e
		}
		return writeJSON(output, v)
	}
	head, e := refreshHead(ctx, s)
	if e != nil {
		return e
	}
	switch o.command {
	case "fighters":
		after := uint32(0)
		for pages := 0; pages < 8192; pages++ {
			page, e := s.Fighters(ctx, head, after)
			if e != nil {
				return e
			}
			for _, item := range page.Items {
				v := struct {
					ProspectID   string `json:"prospect_id"`
					StateID      string `json:"state_id"`
					GymPublicKey string `json:"gym_public_key"`
					Generation   uint32 `json:"generation"`
				}{item.State.ProspectID, item.State.StateID, item.State.OwnerPublicKey, item.State.Generation}
				if e = writeJSON(output, v); e != nil {
					return e
				}
			}
			if page.Complete {
				return writeJSON(output, struct {
					Head     fighterownership.HeadRef `json:"head"`
					Complete bool                     `json:"complete"`
				}{head, true})
			}
			if page.Next <= after {
				return fightercarrier.ErrCorrupt
			}
			after = page.Next
		}
		return fightercarrier.ErrCapacity
	case "discover":
		if _, e = s.BeginRecoveryDiscovery(ctx); e != nil {
			return e
		}
		for step := 0; step < 4096*65+1; step++ {
			v, e := s.RecoveryDiscoveryStep(ctx)
			if e != nil {
				return e
			}
			if v.Result != nil || v.Complete {
				if e = writeJSON(output, v); e != nil {
					return e
				}
			}
			if v.Complete {
				return nil
			}
		}
		return fightercarrier.ErrCapacity
	}
	if e = s.WaitSynchronized(ctx); e != nil {
		return e
	}
	if o.command == "deliver" {
		review, e := s.ReviewDelivery(ctx, o.request, o.leg)
		if e != nil {
			return e
		}
		if e = writeJSON(output, review); e != nil {
			return e
		}
		if e = confirm(ctx, input, reader, prompt, review.ReviewID, review.ExpiresAtMS); e != nil {
			return e
		}
		v, e := s.Deliver(ctx, review.ReviewID)
		if e != nil {
			return e
		}
		return writeJSON(output, v)
	}
	var review pilot.RestoredTransferReview
	switch o.command {
	case "transfer":
		if e = selectedID(&o, output); e == nil {
			review, e = s.PrepareTransfer(ctx, o.request, o.prospect, o.recipient)
		}
	case "recover":
		if e = selectedID(&o, output); e == nil {
			review, e = recoveryReview(ctx, s, o)
		}
	case "resume-action":
		status, x := s.Action(ctx, o.request)
		if x != nil {
			return x
		}
		if status.Kind != "transfer" && status.Stage == "intent" {
			o.commit, o.kind = status.CommitTxID, status.Kind
			review, e = recoveryReview(ctx, s, o)
		} else {
			review, e = s.ResumeAction(ctx, o.request, o.prospect)
		}
	default:
		return fightercarrier.ErrInvalid
	}
	if e != nil {
		return e
	}
	if e = writeJSON(output, review); e != nil {
		return e
	}
	if e = confirm(ctx, input, reader, prompt, review.ReviewID, review.ExpiresAtMS); e != nil {
		return e
	}
	v, e := s.ConfirmAction(ctx, review.ReviewID)
	if e != nil {
		return e
	}
	return writeJSON(output, v)
}
func errorCode(e error) string {
	if errors.Is(e, fightercarrier.ErrOutcomeUnknown) {
		return "outcome_unknown"
	}
	var cleanup *pilot.RestoredGymCleanupError
	var files *pilot.RestoredGymFileCleanupError
	if errors.As(e, &cleanup) || errors.As(e, &files) || errors.Is(e, pilot.ErrRestoredGymStorage) {
		return "storage_failed"
	}
	switch {
	case errors.Is(e, errArguments), errors.Is(e, fightercarrier.ErrInvalid):
		return "invalid_request"
	case errors.Is(e, context.Canceled):
		return "cancelled"
	case errors.Is(e, context.DeadlineExceeded):
		return "timed_out"
	case errors.Is(e, fighterownership.ErrEscrowSpent):
		return "escrow_currently_absent"
	case errors.Is(e, fightercarrier.ErrNotFound):
		return "not_found"
	case errors.Is(e, fightercarrier.ErrUnauthorized):
		return "approval_required"
	case errors.Is(e, fightercarrier.ErrConflict), errors.Is(e, fighterownership.ErrStale):
		return "conflict"
	case errors.Is(e, fightercarrier.ErrBusy), errors.Is(e, fighterownership.ErrBusy):
		return "busy"
	default:
		return "unavailable"
	}
}

// Keep cancellation ownership armed through the final closed diagnostic, too.
// Stopping it earlier would let a full stderr strand an ordinary failure.
func finishCommand(ctx context.Context, cause error, output *os.File, joinOutputs func() error) error {
	e := errors.Join(cause, ctx.Err())
	if e != nil {
		e = errors.Join(e, writeJSON(output, struct {
			Error string `json:"error"`
		}{errorCode(e)}))
	}
	return errors.Join(e, joinOutputs(), ctx.Err())
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	joinOutputs := interruptOutputs(ctx, os.Stdout, os.Stderr)

	e := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if finishCommand(ctx, e, os.Stderr, joinOutputs) != nil {
		os.Exit(1)
	}
}
