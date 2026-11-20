//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// OwnedNoonAuditForTest reports concrete component reopening only. It is not
// a session, wallet, ordinary app reopen, or permission to issue more fighters.
type OwnedNoonAuditForTest struct {
	Head     fighterownership.HeadRef
	Event    gymevent.OwnedInfo
	Cutoff   fighterownership.CutoffRecord
	Counters OwnedFixtureCounters
}

// AuditOwnedNoonComponentsForTest must run only after the original controller
// joined, while the harness owns its original outer instance lock. The caller
// supplies the independently retained complete config and live event receipt;
// files may corroborate those pins but cannot choose replacement authorities.
// Only registry/core/normal-node component owners reopen. Clocks are installed
// immediately after structural Open, before any clock-sensitive replay. Events
// open physically read-only with their original complete stored-state audit.
func AuditOwnedNoonComponentsForTest(ctx context.Context, shared, binary string, expected OwnedLeagueConfig, offset time.Duration, before gymevent.OwnedInfo, record gymevent.OwnedSnapshotReceipt, counters OwnedFixtureCounters) (out OwnedNoonAuditForTest, resultErr error) {
	if wire.TestPoW || ctx == nil || ctx.Err() != nil || offset <= 0 || offset > 8*24*time.Hour || ValidateOwnedLeagueConfig(expected) != nil || !filepath.IsAbs(shared) || filepath.Clean(shared) != shared || !filepath.IsAbs(binary) || filepath.Clean(binary) != binary {
		return out, ErrPoolRuntime
	}
	resolved, e := filepath.EvalSymlinks(shared)
	if e != nil || resolved != shared {
		return out, ErrPoolRuntime
	}
	if before.StoreID != expected.EventStoreID || !reflect.DeepEqual(before.Pin, expected.EventPin) || before.EventCount != 1 || record.Input.Cutoff.CutoffMS <= 0 {
		return out, ErrPoolRuntime
	}
	if e = readPoolRuntimeRecord(filepath.Join(shared, ownedSourceConfigName), expected.Source); e != nil {
		return out, e
	}
	if e = readPoolRuntimeRecord(filepath.Join(shared, poolRuntimeSourceName), expected.PoolSource); e != nil {
		return out, e
	}
	var intent ownedLeagueIntent
	if e = ownedLeagueRead(filepath.Join(shared, ownedLeagueDirectory, ownedLeagueIntentName), &intent); e != nil || !reflect.DeepEqual(intent, ownedLeagueIntent{"slug-owned-league", 1, expected}) {
		return out, ErrPoolRuntime
	}
	// No new namespace, missing component or alternate chain is created.
	for _, name := range []string{"issuer", ownedSourceDirectory, ownedLeagueDirectory, "primary-chain", "peer-chain"} {
		st, x := os.Lstat(filepath.Join(shared, name))
		if x != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return out, ErrPoolRuntime
		}
	}
	work, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	clock := func() time.Time { return time.Now().Add(offset) }
	issuer, e := registry.OpenPublicManifestAuthority(filepath.Join(shared, "issuer"), expected.Source.Ownership)
	if e != nil {
		return out, e
	}
	defer func() { resultErr = errors.Join(resultErr, issuer.Close()) }()
	publisher, e := issuer.PublicPublication(expected.Source.Ownership)
	if e != nil {
		return out, e
	}
	defer func() { resultErr = errors.Join(resultErr, publisher.Close()) }()
	if e = publisher.UseAuthorityClockForTest(work, expected.Source.Ownership, clock); e != nil {
		return out, e
	}
	core, e := fighterownership.Open(filepath.Join(shared, ownedSourceDirectory), expected.Source.Ownership, expected.Source.Store)
	if e != nil {
		return out, e
	}
	defer func() { resultErr = errors.Join(resultErr, core.Close()) }()
	if e = core.UseCalendarClockForTest(work, expected.Source.Ownership, expected.Source.Store, clock); e != nil {
		return out, e
	}
	count := func() (OwnedFixtureCounters, error) {
		info, x := issuer.Info()
		if x != nil {
			return OwnedFixtureCounters{}, x
		}
		p, x := publisher.Status(work)
		if x != nil {
			return OwnedFixtureCounters{}, x
		}
		if info.Pin != expected.Source.Ownership.Source {
			return OwnedFixtureCounters{}, ErrPoolRuntime
		}
		return OwnedFixtureCounters{info.Records, info.Jobs, p.AcceptedSequence, p.SignedAdmissionSequence}, nil
	}
	if out.Counters, e = count(); e != nil || out.Counters != counters {
		return out, errors.Join(ErrPoolRuntime, e)
	}
	// Reuse the exact normal node launch/stop leaves, without a fabricated runtime,
	// account or legacy store handle. Ports are fresh; existing chain DBs are used.
	var ports [4]uint16
	seen := map[uint16]bool{expected.Source.PublicReadPort: true}
	for n := range ports {
		for attempt := 0; attempt < 16; attempt++ {
			p, x := unusedPort()
			if x != nil {
				return out, x
			}
			if !seen[p] {
				ports[n] = p
				seen[p] = true
				break
			}
		}
		if ports[n] == 0 {
			return out, ErrPoolRuntime
		}
	}
	peer, e := startNodeProfile(binary, shared, "peer", ports[0], ports[1], ports[3], true, false)
	if e != nil {
		return out, e
	}
	defer func() { resultErr = errors.Join(resultErr, peer.stop()) }()
	primary, e := startNodeProfile(binary, shared, "primary", ports[2], 0, ports[1], false, true)
	if e != nil {
		return out, e
	}
	defer func() { resultErr = errors.Join(resultErr, primary.stop()) }()
	if e = waitNode(work, primary, true); e != nil {
		return out, e
	}
	if e = waitNode(work, peer, false); e != nil {
		return out, e
	}
	nodes := &pilotNodes{primary: primary, peer: peer}
	node, e := fighterownership.NewOwnedChainSource(work, expected.Source.Ownership, nodes.rpcConfig(), work)
	if e != nil {
		return out, e
	}
	defer func() { resultErr = errors.Join(resultErr, node.Close()) }()
	source, e := fighterownership.NewPublicSource(work, expected.Source.Ownership, publisher)
	if e != nil {
		return out, e
	}
	// Restart produces a new real node instance/revision. Record that authenticated
	// observation and replay it; never substitute the old cached complete head.
	source, _, e = core.RecordChainObservation(work, source, node)
	if e != nil {
		return out, e
	}
	replay, e := core.BeginReplay(work, source, node)
	if e == fighterownership.ErrStale {
		partial, x := core.InspectReplay(work)
		if x != nil {
			return out, x
		}
		if x = core.AbandonReplay(work, partial.WorkID); x != nil {
			return out, x
		}
		replay, e = core.BeginReplay(work, source, node)
	}
	if e != nil {
		return out, e
	}
	defer func() { resultErr = errors.Join(resultErr, replay.Close()) }()
	ready := false
	for step := 0; step < 4096; step++ {
		p, x := replay.Step(work)
		if x != nil {
			return out, x
		}
		if p.Ready {
			if p.Head == nil {
				return out, ErrPoolRuntime
			}
			out.Head = *p.Head
			ready = true
			break
		}
	}
	if !ready {
		return out, fighterownership.ErrCapacity
	}
	if e = replay.Close(); e != nil {
		return out, e
	}
	out.Cutoff, e = core.Cutoff(work, record.Input.Event.EventID)
	if e != nil || !reflect.DeepEqual(out.Cutoff, record.Input.Cutoff) {
		return out, errors.Join(ErrPoolRuntime, e)
	}
	events, e := gymevent.OpenOwnedAudit(work, filepath.Join(shared, ownedLeagueDirectory, "events"), expected.EventPin, core)
	if e != nil {
		return out, e
	}
	defer func() { resultErr = errors.Join(resultErr, events.Close()) }()
	out.Event, e = events.Info(work)
	if e != nil || !reflect.DeepEqual(out.Event, before) {
		return out, errors.Join(ErrPoolRuntime, e)
	}
	// OpenOwnedAudit audited every retained immutable receipt/participant/cutoff
	// against this exact replayed source. No sessionless new record read is added.
	if out.Counters, e = count(); e != nil || out.Counters != counters {
		return out, errors.Join(ErrPoolRuntime, e)
	}
	return out, work.Err()
}
