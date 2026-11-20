// Copyright (c) 2026 The SLUG developers. ISC license.
package walletwithdraw

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/taytr-llc/slug/wire/v2"
)

type Journal struct {
	mu                          sync.Mutex
	identity                    Identity
	binding                     string
	state                       state
	chain                       *dirChain
	lock, markerFile, stateFile *os.File
	markerStamp, stateStamp     fileStamp
	ctx                         context.Context
	cancel                      context.CancelFunc
	closed                      atomic.Bool
	closeOnce                   sync.Once
	closeDone                   chan struct{}
	closeErr                    error
	poison                      error
	now                         func() int64
	syncFile                    func(*os.File) error
	rename                      func(*os.File, string, string) error
}
type Owner struct {
	j        *Journal
	ctx      context.Context
	cancel   context.CancelFunc
	stop     func() bool
	closed   bool
	pending  *PreparedCommit
	delivery *Delivery
}

func Create(ctx context.Context, path string, i Identity) (*Journal, error) {
	return openJournal(ctx, path, i, true)
}
func Open(ctx context.Context, path string, i Identity) (*Journal, error) {
	return openJournal(ctx, path, i, false)
}
func openJournal(ctx context.Context, path string, i Identity, create bool) (j *Journal, err error) {
	return openJournalMode(ctx, path, i, create, false)
}
func openJournalMode(ctx context.Context, path string, i Identity, create, auditOnly bool) (j *Journal, err error) {
	if wire.TestPoW || !supportedPlatform() {
		return nil, ErrProfile
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	binding, e := BindingID(i)
	if e != nil {
		return nil, e
	}
	c, e := pinDirectory(path, create)
	if e != nil {
		return nil, e
	}
	lock, e := lockAt(c.dir(), create)
	if e != nil {
		_ = c.close()
		return nil, e
	}
	life, cancel := context.WithCancel(ctx)
	j = &Journal{identity: i, binding: binding, chain: c, lock: lock, ctx: life, cancel: cancel, closeDone: make(chan struct{}), now: func() int64 { return time.Now().UnixMilli() }, syncFile: func(f *os.File) error { return f.Sync() }, rename: renameAt}
	allocated := j
	defer func() {
		if err != nil {
			cancel()
			err = errors.Join(err, allocated.closeFiles())
		}
	}()
	entries, e := c.dir().ReadDir(6)
	if e != nil && !errors.Is(e, io.EOF) {
		return nil, ErrStorage
	}

	if len(entries) > 4 {
		return nil, ErrCorrupt
	}
	var stage string
	hasMarker, hasState := false, false
	for _, v := range entries {
		n := v.Name()
		switch n {
		case "LOCK":
		case markerName:
			hasMarker = true
		case stateName:
			hasState = true
		default:
			if stage != "" || !validStageName(n) {
				return nil, ErrCorrupt
			}
			stage = n
		}
	}
	expected := markerFor(i, binding)
	empty := state{application, 1, i.JournalID, binding, 0, []Operation{}}
	if !hasMarker {
		if !create || stage != "" {
			return nil, ErrCorrupt
		}
		if hasState {
			f, b, st, e := readAt(c.dir(), stateName, MaxStateBytes)
			if e != nil {
				return nil, ErrCorrupt
			}
			j.stateFile, j.stateStamp = f, st
			if decode(b, MaxStateBytes, &j.state) != nil || !same(j.state, empty) {
				return nil, ErrCorrupt
			}
		} else {
			if e = writeInitial(c.dir(), stateName, empty); e != nil {
				return nil, e
			}
		}
		if e = writeInitial(c.dir(), markerName, expected); e != nil {
			return nil, e
		}
		hasMarker, hasState = true, true
	}
	if !hasState {
		return nil, ErrCorrupt
	}
	f, b, st, e := readAt(c.dir(), markerName, 4096)
	if e != nil {
		return nil, ErrCorrupt
	}
	j.markerFile, j.markerStamp = f, st
	var m marker
	if decode(b, 4096, &m) != nil || m != expected {
		return nil, ErrCorrupt
	}
	if j.stateFile == nil {
		f, b, st, e = readAt(c.dir(), stateName, MaxStateBytes)
		if e != nil {
			return nil, ErrCorrupt
		}
		j.stateFile, j.stateStamp = f, st
		if decode(b, MaxStateBytes, &j.state) != nil {
			return nil, ErrCorrupt
		}
	}
	if e = validateState(ctx, j.state, i, binding); e != nil {
		return nil, e
	}
	if create && len(j.state.Records) != 0 {
		return nil, ErrConflict
	}
	if stage != "" {
		s, e := relativeStamp(c.dir(), stage)
		if e != nil || !privateFile(s) {
			return nil, ErrCorrupt
		}
		if !auditOnly && (removeAt(c.dir(), stage) != nil || c.dir().Sync() != nil) {
			return nil, ErrStorage
		}
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return j, nil
}
func validStageName(n string) bool {
	return strings.HasPrefix(n, ".state-") && strings.HasSuffix(n, ".tmp") && hexValue(strings.TrimSuffix(strings.TrimPrefix(n, ".state-"), ".tmp"), 32, true)
}
func writeInitial(d *os.File, name string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return ErrInvalid
	}
	f, e := createAt(d, name)
	if e != nil {
		return ErrStorage
	}
	_, we := f.Write(b)
	se := f.Sync()
	ce := f.Close()
	de := d.Sync()
	if we != nil || se != nil || ce != nil || de != nil {
		return errors.Join(ErrOutcomeUnknown, ErrStorage)
	}
	return nil
}
func (j *Journal) Identity() Identity {
	if j == nil {
		return Identity{}
	}
	return j.identity
}
func (j *Journal) BindingID() string {
	if j == nil {
		return ""
	}
	return j.binding
}
func (j *Journal) TryAcquire(ctx context.Context) (*Owner, error) {
	if j == nil || j.closed.Load() {
		return nil, ErrClosed
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if !j.mu.TryLock() {
		return nil, ErrBusy
	}
	if j.closed.Load() || j.ctx.Err() != nil {
		j.mu.Unlock()
		return nil, ErrClosed
	}
	if j.poison != nil {
		e := j.poison
		j.mu.Unlock()
		return nil, e
	}
	life, cancel := context.WithCancel(ctx)
	o := &Owner{j: j, ctx: life, cancel: cancel}
	o.stop = context.AfterFunc(j.ctx, cancel)
	return o, nil
}
func (o *Owner) check() error {
	if o == nil || o.closed || o.j.closed.Load() {
		return ErrClosed
	}
	if e := o.ctx.Err(); e != nil {
		return e
	}
	if e := o.j.ctx.Err(); e != nil {
		return ErrClosed
	}
	return o.j.poison
}
func (o *Owner) Snapshot() (Snapshot, error) {
	if e := o.check(); e != nil {
		return Snapshot{}, e
	}
	s := o.j.state
	r := Snapshot{JournalID: s.JournalID, BindingID: s.BindingID, Count: uint32(len(s.Records)), ClockHighWaterMS: s.ClockHighWaterMS}
	for _, v := range s.Records {
		r.LatestOperationID = v.OperationID
		if !done(v) {
			r.UnresolvedOperationID = v.OperationID
		}
	}
	return r, nil
}
func (o *Owner) find(id string) (int, error) {
	if e := o.check(); e != nil {
		return 0, e
	}
	if !hexValue(id, 32, true) {
		return 0, ErrInvalid
	}
	for n, v := range o.j.state.Records {
		if v.OperationID == id {
			return n, nil
		}
	}
	return 0, ErrNotFound
}
func (o *Owner) Lookup(id string) (Operation, error) {
	n, e := o.find(id)
	if e != nil {
		return Operation{}, e
	}
	return ownedOperation(o.j.state.Records[n]), nil
}
func (o *Owner) List(after, limit uint32) ([]Operation, error) {
	if e := o.check(); e != nil {
		return nil, e
	}
	if limit < 1 || limit > 16 || after > uint32(len(o.j.state.Records)) {
		return nil, ErrInvalid
	}
	out := make([]Operation, 0, limit)
	for _, v := range o.j.state.Records {
		if v.Sequence > after {
			out = append(out, ownedOperation(v))
			if len(out) == int(limit) {
				break
			}
		}
	}
	return out, nil
}
func (o *Owner) Close() error {
	if o == nil || o.closed {
		return nil
	}
	var err error
	if o.pending != nil {
		err = o.pending.Close()
	}
	o.closed = true
	o.cancel()
	o.stop()
	o.delivery = nil
	o.j.mu.Unlock()
	return err
}
func (j *Journal) closeFiles() error {
	var e error
	for _, f := range []*os.File{j.stateFile, j.markerFile, j.lock} {
		if f != nil {
			e = errors.Join(e, f.Close())
		}
	}
	if j.chain != nil {
		e = errors.Join(e, j.chain.close())
	}
	return e
}
func (j *Journal) Close() error {
	if j == nil {
		return nil
	}
	j.closeOnce.Do(func() {
		j.closed.Store(true)
		j.cancel()
		j.mu.Lock()
		j.closeErr = errors.Join(j.poison, j.closeFiles())
		j.mu.Unlock()
		close(j.closeDone)
	})
	<-j.closeDone
	return j.closeErr
}
