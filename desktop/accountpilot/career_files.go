// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/internal/careerstate"
)

type careerAccountMarker struct {
	resultAccountMarker
	CareerSchema     uint32                  `json:"career_schema"`
	CareerPhase      string                  `json:"career_phase"`
	CareerActivation *careerstate.Activation `json:"career_activation"`
}

func careerBase(m careerAccountMarker) resultAccountMarker {
	b := m.resultAccountMarker
	b.Version = 8
	return b
}
func validateCareerMarker(m careerAccountMarker) error {
	if m.Version != 9 || m.CareerSchema != 1 {
		return pilot.ErrManifestInitialization
	}
	if _, e := validateResultAccountMarker(careerBase(m)); e != nil {
		return e
	}
	switch m.CareerPhase {
	case "waiting_source":
		if m.CareerActivation != nil {
			return pilot.ErrManifestInitialization
		}
	case "prepared", "ready":
		if m.ResultPhase != "ready" {
			return pilot.ErrManifestInitialization
		}
		if m.CareerActivation == nil {
			if m.CareerPhase == "ready" {
				return pilot.ErrManifestInitialization
			}
			return nil
		}
		if careerstate.ValidateActivation(*m.CareerActivation) != nil || m.CareerActivation.ResultStoreID != m.ResultStoreID {
			return pilot.ErrManifestInitialization
		}
	default:
		return pilot.ErrManifestInitialization
	}
	return nil
}
func readCareerMarker(path string) (careerAccountMarker, error) {
	var m careerAccountMarker
	before, e := os.Lstat(path)
	if e != nil || !before.Mode().IsRegular() || before.Size() > 16384 {
		return m, pilot.ErrManifestInitialization
	}
	f, e := os.Open(path)
	if e != nil {
		return m, e
	}
	defer f.Close()
	after, e := f.Stat()
	if e != nil || !os.SameFile(before, after) {
		return m, pilot.ErrManifestInitialization
	}
	raw, e := io.ReadAll(io.LimitReader(f, 16385))
	if e != nil || len(raw) > 16384 || json.Unmarshal(raw, &m) != nil {
		return m, pilot.ErrManifestInitialization
	}
	canonical, _ := json.Marshal(m)
	if !bytes.Equal(raw, append(canonical, '\n')) {
		return m, pilot.ErrManifestInitialization
	}
	return m, validateCareerMarker(m)
}

// Only the explicit career constructor may read either predecessor or v9.
func readCareerResultMarker(path string) (resultAccountMarker, error) {
	if m, e := readCareerMarker(path); e == nil {
		return careerBase(m), nil
	}
	return readResultMarker(path)
}
func writeCareerMarker(path string, m careerAccountMarker) error {
	if e := validateCareerMarker(m); e != nil {
		return e
	}
	b, e := json.Marshal(m)
	if e != nil || len(b)+1 > 16384 {
		return pilot.ErrManifestInitialization
	}
	return writeMarker(path, append(b, '\n'))
}
func (c *StableController) readResultRoot(path string) (resultAccountMarker, error) {
	if c.careerOwner != nil {
		return readCareerResultMarker(path)
	}
	return readResultMarker(path)
}
func (c *StableController) writeResultRoot(path string, b resultAccountMarker) error {
	if c.careerOwner == nil {
		return writeResultMarker(path, b)
	}
	m, e := readCareerMarker(path)
	if e != nil {
		if _, e = readResultMarker(path); e != nil {
			return e
		}
		return writeResultMarker(path, b)
	}
	// Preserve the predecessor writer's atomic second-participant transition.
	if b.EventPhase == "waiting_participants" {
		if pin, e := seedEventAccountPin(cardBase(boutBase(resultBase(b)))); e == nil {
			b.EventPhase = "prepared"
			b.EventStoreID, _ = gymevent.ID(pin)
		}
	}
	b.Version = 9
	m.resultAccountMarker = b
	return writeCareerMarker(path, m)
}
func (c *StableController) ensureCareers(ctx context.Context, child *pilot.StableController) error {
	o := c.careerOwner
	if o == nil {
		return nil
	}
	path := filepath.Join(c.config.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	m, e := readCareerMarker(path)
	if e != nil {
		b, be := readResultMarker(path)
		if be != nil {
			return be
		}
		b.Version = 9
		m = careerAccountMarker{b, 1, "waiting_source", nil}
	}
	if m.ResultPhase != "ready" {
		return writeCareerMarker(path, m)
	}
	dir := filepath.Join(c.config.DataDir, "results")
	if c.boutOwner.store == nil {
		return pilot.ErrManifestInitialization
	}
	bi, e := c.boutOwner.store.Info(ctx)
	if e != nil {
		return e
	}
	pin, e := gymresult.CandidatePin(bi.Pin)
	if e != nil {
		return e
	}
	if m.CareerPhase == "waiting_source" {
		// No child operations are published yet. Release only the Result handle so
		// eligibility is audited under the real DB lock before the v9 intent.
		if c.resultOwner.store != nil {
			if e = c.resultOwner.store.Close(); e != nil {
				return e
			}
			c.resultOwner.store = nil
		}
		if e = o.deps.preflight(ctx, dir, pin, c.boutOwner.store); e != nil {
			return e
		}
		m.CareerPhase = "prepared"
		if e = writeCareerMarker(path, m); e != nil {
			return e
		}
		c.careerMark("career-prepared")
	}
	if c.resultOwner.store == nil {
		if m.CareerPhase == "ready" {
			c.resultOwner.store, e = o.deps.openResult(ctx, dir, pin, c.boutOwner.store)
		} else {
			c.resultOwner.store, e = o.deps.resumeResult(ctx, dir, pin, c.boutOwner.store)
		}
		if e != nil {
			return e
		}
	}
	activation, e := c.resultOwner.store.CareerInfo(ctx)
	if e != nil {
		return e
	}
	if m.CareerActivation == nil {
		if m.CareerPhase != "prepared" {
			return pilot.ErrManifestInitialization
		}
		m.CareerActivation = &activation
		if e = writeCareerMarker(path, m); e != nil {
			return e
		}
		c.careerMark("career-activation")
	} else if *m.CareerActivation != activation {
		return pilot.ErrManifestInitialization
	}
	reader, e := c.resultOwner.store.CareerReader()
	if e != nil {
		return e
	}
	// All bindings are concrete local handles, before admitting this child.
	for _, x := range m.StableInstalls {
		if x.Phase != "ready" {
			return pilot.ErrManifestInitialization
		}
		if child != nil {
			cp, ce := child.StablePin()
			if ce != nil {
				return ce
			}
			if cp == x.Pin {
				if e = child.BindCareer(ctx, reader, o.deps.openStable); e != nil {
					return e
				}
				c.careerMark("career-stable-bound")
				continue
			}
		}
		source, e := openBoutStartupSource(ctx, filepath.Join(c.config.DataDir, "gyms", x.AccountID, "manifest-authority"), x.Pin.Source)
		if e != nil {
			return e
		}
		var stableErr error
		open := o.deps.resumeStable
		if m.CareerPhase == "ready" {
			open = o.deps.openStable
		}
		stable, e := open(filepath.Join(c.config.DataDir, "gyms", x.AccountID, "stable"), x.Pin, source, activation)
		if e == nil {
			if m.CareerPhase == "ready" {
				e = stable.AttachCareer(ctx, reader)
			} else {
				e = stable.BindCareer(ctx, reader)
			}
			stableErr = stable.Close()
		}
		e = errors.Join(e, stableErr, source.Close())
		if e != nil {
			return e
		}
		c.careerMark("career-stable-bound")
	}
	if c.cardOwner.store == nil {
		return pilot.ErrManifestInitialization
	}
	if m.CareerPhase == "ready" {
		e = c.cardOwner.store.AttachCareer(ctx, reader)
	} else {
		e = c.cardOwner.store.BindCareer(ctx, reader)
	}
	if e != nil {
		return e
	}
	c.careerMark("career-card-bound")
	if m.CareerPhase != "ready" {
		m.CareerPhase = "ready"
		if e = writeCareerMarker(path, m); e != nil {
			return e
		}
		c.careerMark("career-ready")
	}
	o.reader = reader
	return nil
}
func (c *StableController) careerMark(stage string) {
	if c.checkpoint != nil {
		c.checkpoint(stage)
	}
}
