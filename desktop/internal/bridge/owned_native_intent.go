// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

// OwnedNativeIntent preserves public correlation before Prepare can publish
// anything in the helper. It contains no signature or consent and cannot be
// used to reconstruct a live review. A later login must explicitly Resume.
type OwnedNativeIntent struct {
	Version            uint32 `json:"version"`
	RequestID          string `json:"request_id"`
	CreatedAtMS        int64  `json:"created_at_ms"`
	SourceBindingID    string `json:"source_binding_id"`
	OwnershipPinID     string `json:"ownership_pin_id"`
	CarrierBindingID   string `json:"carrier_binding_id"`
	AccountID          string `json:"account_id"`
	OriginalViewID     string `json:"original_view_id"`
	OriginalGeneration uint64 `json:"original_generation"`
	ProspectID         string `json:"prospect_id"`
	RecipientGym       string `json:"recipient_gym"`
}

// The root is a native-selected sibling of the helper's data root. Records
// are immutable and bounded to both fixture journals' 128-request capacity.
// The helper's exclusive selected-root owner prevents a second live writer.
type OwnedIntentLog struct {
	mu                sync.Mutex
	dir               string
	parent            recoveryfile.Identity
	rows              map[string]OwnedNativeIntent
	closed, uncertain bool
	cleanup           []io.Closer
	cleanupErr        error
}

func OpenOwnedIntentLog(ctx context.Context, dir string) (*OwnedIntentLog, error) {
	if ctx == nil || ctx.Err() != nil || !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, ErrUnavailable
	}
	l := &OwnedIntentLog{dir: dir, rows: map[string]OwnedNativeIntent{}}
	created := false
	if e := os.Mkdir(dir, 0700); e != nil && !os.IsExist(e) {
		return nil, ErrUnavailable
	} else {
		created = e == nil
	}
	st, e := os.Lstat(dir)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0077 != 0 {
		return nil, ErrUnavailable
	}
	// Capture's descriptor walk rejects substituted or symlink ancestors.
	probe, e := recoveryfile.Capture(filepath.Join(dir, "native-probe.json"), nil, recoveryfile.CreateMode)
	if e != nil {
		return nil, ErrUnavailable
	}
	parentIdentity := probe.Pin().Parent
	if e = l.finishClose(probe); e != nil {
		return l, e
	}
	if created {
		parent, e := os.Open(filepath.Dir(dir))
		if e != nil {
			return nil, e
		}
		e = errors.Join(parent.Sync(), l.finishClose(parent))
		if e != nil {
			if l.cleanupErr != nil {
				return l, e
			}
			return nil, e
		}
	}
	f, e := os.Open(dir)
	if e != nil {
		return nil, e
	}
	entries, readErr := f.ReadDir(257)
	closeErr := l.finishClose(f)
	if closeErr != nil {
		return l, closeErr
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil || len(entries) > 256 {
		return nil, ErrUnavailable
	}
	l.parent = parentIdentity
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || !ownedcodec.Hex(strings.TrimSuffix(name, ".json"), 32) {
			return nil, ErrUnavailable
		}
		p := filepath.Join(dir, name)
		selection, e := recoveryfile.Capture(p, nil, recoveryfile.ReadMode)
		if e != nil {
			return nil, ErrUnavailable
		}
		if selection.Pin().Parent != parentIdentity {
			if e = l.finishClose(selection); e != nil {
				return l, e
			}
			return nil, ErrUnavailable
		}
		h, e := recoveryfile.Open(selection.Pin(), nil)
		if e != nil {
			if e = l.finishClose(selection); e != nil {
				return l, e
			}
			return nil, ErrUnavailable
		}
		raw, readErr := h.Read(ctx, func(fn func(func() error) error) error { return fn(ctx.Err) })
		e = errors.Join(readErr, l.finishClose(h, selection))
		if l.cleanupErr != nil {
			return l, e
		}
		if e != nil {
			return nil, ErrUnavailable
		}
		var row OwnedNativeIntent
		if ownedcodec.Decode(raw, 4096, &row) != nil || !validOwnedNativeIntent(row) || row.RequestID+".json" != name {
			return nil, ErrUnavailable
		}
		l.rows[row.RequestID] = row
	}
	return l, nil
}

func validOwnedNativeIntent(r OwnedNativeIntent) bool {
	if r.Version != 1 || r.CreatedAtMS <= 0 || r.CreatedAtMS > 253402300799999 || r.OriginalGeneration == 0 || r.OriginalGeneration > 9007199254740991 {
		return false
	}
	for _, s := range []string{r.RequestID, r.SourceBindingID, r.OwnershipPinID, r.CarrierBindingID, r.AccountID, r.OriginalViewID, r.ProspectID} {
		if !ownedcodec.Hex(s, 32) || s == strings.Repeat("0", 64) {
			return false
		}
	}
	return pilot.ValidateOwnedGymRequest(pilot.OwnedGymRequest{Version: 1, ID: 1, Command: "transfer_prepare", ViewID: r.OriginalViewID, RequestID: r.RequestID, ProspectID: r.ProspectID, RecipientGym: r.RecipientGym}) == nil
}

func ownedIntentMatches(r OwnedNativeIntent, s pilot.OwnedGymScope) bool {
	return r.SourceBindingID == s.SourceBindingID && r.OwnershipPinID == s.Carrier.Pin.OwnershipPinID && r.CarrierBindingID == s.CarrierBindingID && r.AccountID == s.Carrier.Pin.AccountID
}

func (l *OwnedIntentLog) Prepare(ctx context.Context, original *OwnedContext, prospect, recipient string) (OwnedNativeIntent, error) {
	if l == nil || ctx == nil || original == nil {
		return OwnedNativeIntent{}, ErrUnavailable
	}
	if !l.mu.TryLock() {
		return OwnedNativeIntent{}, ErrBusy
	}
	defer l.mu.Unlock()
	if l.closed || l.uncertain || len(l.rows) >= 256 || !original.helper.ContextMatches(original) {
		return OwnedNativeIntent{}, ErrUnavailable
	}
	count := 0
	for _, row := range l.rows {
		if ownedIntentMatches(row, original.scope) {
			count++
		}
	}
	if count >= 128 {
		return OwnedNativeIntent{}, &OwnedFailure{Code: "capacity"}
	}
	var id [32]byte
	if _, e := rand.Read(id[:]); e != nil {
		return OwnedNativeIntent{}, e
	}
	r := OwnedNativeIntent{Version: 1, RequestID: hex.EncodeToString(id[:]), CreatedAtMS: time.Now().UnixMilli(), SourceBindingID: original.scope.SourceBindingID, OwnershipPinID: original.scope.Carrier.Pin.OwnershipPinID, CarrierBindingID: original.scope.CarrierBindingID, AccountID: original.view.AccountID, OriginalViewID: original.view.ViewID, OriginalGeneration: original.view.Generation, ProspectID: prospect, RecipientGym: recipient}
	req := pilot.OwnedGymRequest{Version: 1, ID: 1, Command: "transfer_prepare", ViewID: r.OriginalViewID, RequestID: r.RequestID, ProspectID: prospect, RecipientGym: recipient}
	if !validOwnedNativeIntent(r) || pilot.ValidateOwnedGymRequest(req) != nil || recipient == original.view.GymPublicKey {
		return OwnedNativeIntent{}, ErrUnavailable
	}
	selection, e := recoveryfile.Capture(filepath.Join(l.dir, r.RequestID+".json"), nil, recoveryfile.CreateMode)
	if e != nil {
		return OwnedNativeIntent{}, e
	}
	if selection.Pin().Parent != l.parent {
		return OwnedNativeIntent{}, errors.Join(ErrUnavailable, l.finishClose(selection))
	}
	h, e := recoveryfile.Open(selection.Pin(), nil)
	if e != nil {
		return OwnedNativeIntent{}, errors.Join(e, l.finishClose(selection))
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return OwnedNativeIntent{}, errors.Join(e, l.finishClose(h, selection))
	}
	check := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !original.helper.ContextMatches(original) {
			return &OwnedFailure{Code: "stale_view"}
		}
		return nil
	}
	e = h.Publish(ctx, raw, func(fn func(func() error) error) error {
		if e := check(); e != nil {
			return e
		}
		return fn(check)
	})
	e = errors.Join(e, l.finishClose(h, selection))
	if e != nil {
		l.uncertain = true
		return OwnedNativeIntent{}, errors.Join(&OwnedFailure{Code: "outcome_unknown"}, e)
	}
	l.rows[r.RequestID] = r
	return r, nil
}

// List and Lookup disclose only original-scope requests. They authorize no
// signing/delivery and never select the newest request to rescue another one.
func (l *OwnedIntentLog) List(original *OwnedContext) ([]OwnedNativeIntent, error) {
	if l == nil || original == nil {
		return nil, ErrUnavailable
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.uncertain || !original.helper.ContextMatches(original) {
		return nil, ErrUnavailable
	}
	rows := []OwnedNativeIntent{}
	for _, r := range l.rows {
		if ownedIntentMatches(r, original.scope) {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAtMS == rows[j].CreatedAtMS {
			return rows[i].RequestID < rows[j].RequestID
		}
		return rows[i].CreatedAtMS < rows[j].CreatedAtMS
	})
	return rows, nil
}
func (l *OwnedIntentLog) Lookup(original *OwnedContext, id string) (OwnedNativeIntent, error) {
	rows, e := l.List(original)
	if e != nil {
		return OwnedNativeIntent{}, e
	}
	for _, r := range rows {
		if r.RequestID == id {
			return r, nil
		}
	}
	return OwnedNativeIntent{}, &OwnedFailure{Code: "not_found"}
}
func (l *OwnedIntentLog) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.cleanupErr != nil {
		return l.cleanupErr
	}
	if l.uncertain {
		return &OwnedFailure{Code: "outcome_unknown"}
	}
	return nil
}

// At most one failing close batch is possible: every subsequent operation
// refuses the poisoned owner. Preserve those exact handles, even if a later
// Close would be a successful no-op. Constructors may return this owner+error.
func (l *OwnedIntentLog) finishClose(owners ...io.Closer) error {
	var closeErr error
	for _, owner := range owners {
		if owner != nil {
			closeErr = errors.Join(closeErr, owner.Close())
		}
	}
	if closeErr != nil {
		l.uncertain = true
		if l.cleanupErr == nil {
			l.cleanup = append([]io.Closer(nil), owners...)
			l.cleanupErr = errors.Join(&OwnedFailure{Code: "outcome_unknown"}, closeErr)
		}
		return l.cleanupErr
	}
	return nil
}
