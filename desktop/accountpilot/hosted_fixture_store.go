// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymprovider"
)

func openHostedDriver(ctx context.Context, path string, cfg gymprovider.Config, accounts *gymaccount.Store, coordinator *gymprovider.Coordinator, create bool) (_ *hostedFixtureDriver, resultErr error) {
	if ctx == nil || ctx.Err() != nil || accounts == nil || coordinator == nil || !hostedID(coordinator.CoordinatorID()) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, gymprovider.ErrConfig
	}
	expected, e := hostedProviderConfig(accounts.AuthorityID())
	if e != nil || expected != cfg {
		return nil, gymprovider.ErrConfig
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(path))
	if e != nil || parent != filepath.Dir(path) {
		return nil, gymprovider.ErrConfig
	}
	if create {
		if e = os.Mkdir(path, 0700); e != nil && !os.IsExist(e) {
			return nil, e
		}
	}
	if !realDirectory(path) {
		return nil, gymprovider.ErrCorrupt
	}
	root, e := os.OpenRoot(path)
	if e != nil {
		return nil, e
	}
	dir, e := root.Open(".")
	if e != nil {
		_ = root.Close()
		return nil, e
	}
	d := &hostedFixtureDriver{root: root, directory: dir, path: path, accounts: accounts, coordinator: coordinator, random: rand.Reader, now: time.Now}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, d.Close())
		}
	}()
	if st, e := root.Lstat("instance-lock"); e == nil {
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return nil, gymprovider.ErrCorrupt
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	d.lock, e = leveldb.OpenFile(filepath.Join(path, "instance-lock"), nil)
	if e != nil {
		return nil, gymprovider.ErrUnavailable
	}
	d.key, e = hostedFixtureKey()
	if e != nil {
		return nil, e
	}
	initial := hostedDriverState{Version: 1, DriverID: hostedDriverID, CoordinatorID: coordinator.CoordinatorID(), Config: cfg, Revision: 1, Epochs: []hostedEpoch{{Fixture: hostedFixtures[0], Subject: hostedFixtures[0], CredentialEpoch: 1}, {Fixture: hostedFixtures[1], Subject: hostedFixtures[1], CredentialEpoch: 1}}, Auth: []hostedAuth{}, Resets: []hostedReset{}}
	entries, e := root.Open(".")
	if e != nil {
		return nil, e
	}
	list, e := entries.ReadDir(4)
	ce := entries.Close()
	if e != nil && e != io.EOF {
		return nil, e
	}
	if ce != nil {
		return nil, ce
	}
	for _, entry := range list {
		if entry.Name() != "instance-lock" && entry.Name() != "state.json" && entry.Name() != ".state-prepared" {
			return nil, gymprovider.ErrCorrupt
		}
	}
	raw, e := d.readState("state.json")
	if os.IsNotExist(e) {
		if !create {
			return nil, gymprovider.ErrCorrupt
		}
		d.state = initial
		if prepared, pe := d.readState(".state-prepared"); pe == nil {
			want, _ := json.Marshal(initial)
			if !bytes.Equal(prepared, append(want, '\n')) {
				return nil, gymprovider.ErrCorrupt
			}
			if e = root.Remove(".state-prepared"); e != nil {
				return nil, e
			}
		} else if !os.IsNotExist(pe) {
			return nil, pe
		}
		raw, _ = json.Marshal(initial)
		if e = d.writeState(append(raw, '\n')); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	} else {
		if json.Unmarshal(raw, &d.state) != nil {
			return nil, gymprovider.ErrCorrupt
		}
		canonical, _ := json.Marshal(d.state)
		if !bytes.Equal(raw, append(canonical, '\n')) || auditHostedDriver(d.state, cfg, coordinator.CoordinatorID(), nil) != nil {
			return nil, gymprovider.ErrCorrupt
		}
		// A prepared file never made visible by rename grants no assertion. Audit it
		// before removing this exact interrupted stage; the committed file wins.
		if prepared, pe := d.readState(".state-prepared"); pe == nil {
			var next hostedDriverState
			if json.Unmarshal(prepared, &next) != nil || auditHostedDriver(next, cfg, coordinator.CoordinatorID(), nil) != nil {
				return nil, gymprovider.ErrCorrupt
			}
			want, _ := json.Marshal(next)
			if !bytes.Equal(prepared, append(want, '\n')) || next.Revision != d.state.Revision+1 {
				return nil, gymprovider.ErrCorrupt
			}
			if e = root.Remove(".state-prepared"); e != nil {
				return nil, e
			}
			if e = dir.Sync(); e != nil {
				return nil, e
			}
		} else if !os.IsNotExist(pe) {
			return nil, pe
		}
	}
	// Account associations are checked against the pinned authority, never adopted
	// from driver rows. This uses only read-only schema2 accessors.
	associations := map[string]string{}
	for _, r := range d.state.Resets {
		if _, ok := associations[r.Fixture]; ok {
			continue
		}
		a, ae := accounts.GetIdentityAccount(ctx, gymprovider.FixtureIssuer, r.Fixture)
		if ae != nil {
			return nil, ae
		}
		associations[r.Fixture] = a.ID
	}
	if e = auditHostedDriver(d.state, cfg, coordinator.CoordinatorID(), associationsForRows(associations, d.state.Resets)); e != nil {
		return nil, e
	}
	return d, nil
}
func associationsForRows(m map[string]string, rows []hostedReset) map[string]string {
	if len(rows) == 0 {
		return nil
	}
	return m
}
func (d *hostedFixtureDriver) readState(name string) ([]byte, error) {
	before, e := d.root.Lstat(name)
	if e != nil {
		return nil, e
	}
	if !before.Mode().IsRegular() || before.Size() > hostedDriverMax {
		return nil, gymprovider.ErrCorrupt
	}
	f, e := d.root.Open(name)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	after, e := f.Stat()
	if e != nil || !os.SameFile(before, after) || !after.Mode().IsRegular() {
		return nil, gymprovider.ErrCorrupt
	}
	raw, e := io.ReadAll(io.LimitReader(f, hostedDriverMax+1))
	if e != nil || len(raw) > hostedDriverMax {
		return nil, gymprovider.ErrCorrupt
	}
	return raw, nil
}
func (d *hostedFixtureDriver) checkDirectory() error {
	a, e := d.directory.Stat()
	if e != nil {
		return e
	}
	b, e := os.Lstat(d.path)
	if e != nil || !b.IsDir() || b.Mode()&os.ModeSymlink != 0 || !os.SameFile(a, b) {
		return gymprovider.ErrCorrupt
	}
	resolved, e := filepath.EvalSymlinks(d.path)
	if e != nil || resolved != d.path {
		return gymprovider.ErrCorrupt
	}
	return nil
}
func (d *hostedFixtureDriver) writeState(raw []byte) (resultErr error) {
	if len(raw) > hostedDriverMax {
		return gymprovider.ErrCapacity
	}
	if e := d.checkDirectory(); e != nil {
		return e
	}
	f, e := d.root.OpenFile(".state-prepared", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	cleanup := true
	defer func() {
		if cleanup {
			e := d.root.Remove(".state-prepared")
			if !os.IsNotExist(e) {
				resultErr = errors.Join(resultErr, e)
			}
		}
	}()
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return e
	}
	if e = d.checkDirectory(); e != nil {
		return e
	}
	if e = d.root.Rename(".state-prepared", "state.json"); e != nil {
		return e
	}
	cleanup = false
	if e = d.directory.Sync(); e != nil {
		return e
	}
	return d.checkDirectory()
}
func (d *hostedFixtureDriver) enter(ctx context.Context) (int64, error) {
	if ctx == nil {
		return 0, gymprovider.ErrConfig
	}
	if e := ctx.Err(); e != nil {
		return 0, e
	}
	if !d.mu.TryLock() {
		return 0, errHostedBusy
	}
	if d.closed || d.poisoned {
		d.mu.Unlock()
		return 0, gymprovider.ErrUnavailable
	}
	now := d.now().UnixMilli()
	if now < 1000 || now > hostedTimeMax-120000 || now < d.state.ClockHighWaterMS {
		d.mu.Unlock()
		return 0, gymprovider.ErrConflict
	}
	if e := d.checkDirectory(); e != nil {
		d.mu.Unlock()
		return 0, e
	}
	return now, nil
}
func (d *hostedFixtureDriver) save(ctx context.Context, next hostedDriverState, now int64) error {
	if d.state.Revision == math.MaxUint32 {
		return gymprovider.ErrCapacity
	}
	next.Revision = d.state.Revision + 1
	next.ClockHighWaterMS = now
	if e := auditHostedDriver(next, d.state.Config, d.state.CoordinatorID, nil); e != nil {
		return e
	}
	raw, e := json.Marshal(next)
	if e != nil || len(raw)+1 > hostedDriverMax {
		return gymprovider.ErrCapacity
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	writer := d.publish
	if writer == nil {
		writer = d.writeState
	}
	if e = writer(append(raw, '\n')); e != nil {
		d.poisoned = true
		return errors.Join(gymprovider.ErrOutcomeUnknown, e)
	}
	d.state = next
	return nil
}
func (d *hostedFixtureDriver) ID() string { return hostedDriverID }
func (d *hostedFixtureDriver) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return d.closeErr
	}
	d.closed = true
	if d.key != nil {
		d.key.D.SetInt64(0)
		d.key = nil
	}
	if d.lock != nil {
		d.closeErr = errors.Join(d.closeErr, d.lock.Close())
	}
	if d.directory != nil {
		d.closeErr = errors.Join(d.closeErr, d.directory.Close())
	}
	if d.root != nil {
		d.closeErr = errors.Join(d.closeErr, d.root.Close())
	}
	return d.closeErr
}
