//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/registry"
)

func TestManifestPackagedHelperNormalPoW(t *testing.T) {
	helper, node := os.Getenv("SLUG_MANIFEST_HELPER"), os.Getenv("SLUG_MANIFEST_NODE")
	if helper == "" || node == "" {
		t.Skip("explicit frozen normal helper and node required")
	}
	root, e := os.MkdirTemp(os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"), "manifest-desktop-")
	if e != nil {
		t.Fatal(e)
	}
	t.Log("public candidate helper artifacts", root)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	data := filepath.Join(root, "data")
	b, e := LaunchManifests(helper, data, node)
	if e != nil {
		t.Fatal(e)
	}
	inventory := newManifestProcessInventory(data, node, b.cmd.Process.Pid)
	cleanupChecked := false
	defer func() {
		if e := b.Close(); e != nil {
			t.Error(e)
		}
		if !cleanupChecked {
			inventory.checkClosed(t)
		}
	}()
	request := func(command string, seq uint64) pilot.ManifestResponse {
		t.Helper()
		r, e := b.Request(ctx, command, seq)
		if e != nil {
			t.Fatal(command, e)
		}
		return r
	}
	wait := func(check func(pilot.ManifestSnapshot) bool) pilot.ManifestSnapshot {
		t.Helper()
		deadline := time.Now().Add(120 * time.Second)
		for time.Now().Before(deadline) {
			s := *request("snapshot", 0).Snapshot
			if s.Account.State == "failed" || s.State == "failed" {
				t.Fatalf("candidate helper failed: account=%s worker=%s", s.Account.LastError, s.LastError)
			}
			if check(s) {
				return s
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(200 * time.Millisecond):
			}
		}
		t.Fatal("candidate state transition timeout")
		return pilot.ManifestSnapshot{}
	}
	initial := *request("snapshot", 0).Snapshot
	if initial.Account.State != "signed_out" {
		t.Fatal("unexpected initial account")
	}
	type saved struct {
		Fixture  string                 `json:"fixture"`
		Receipt  pilot.ManifestReceipt  `json:"receipt"`
		Portrait pilot.ManifestPortrait `json:"portrait"`
	}
	rows := []saved{}
	for _, fixture := range []string{"zero256", "incrementing256"} {
		firstRow := len(rows)
		t.Log("login", fixture)
		if _, e = b.RequestFixture(ctx, fixture); e != nil {
			t.Fatal(e)
		}
		ready := wait(func(s pilot.ManifestSnapshot) bool { return s.Account.State == "signed_in" })
		if ready.Attempts != 0 || ready.AcceptedCount != 0 || ready.State != "stopped" || !ready.Projection.CompleteRecent {
			t.Fatal("login mined or omitted cold verification")
		}
		inventory.capture(t, fixture, "prepared", ready.Account.ID, ready.View.AuthorityOrigin, false)
		request("start", 0)
		mined := wait(func(s pilot.ManifestSnapshot) bool {
			if s.AcceptedCount < 3 || len(s.Projection.Items) < 3 || s.State != "running" {
				return false
			}
			for _, item := range s.Projection.Items {
				if item.EligibilityMode == "canonical-winning-block" && s.Projection.ThroughSequence > item.Sequence {
					return true
				}
			}
			return false
		})
		inventory.capture(t, fixture, "initial-mining", mined.Account.ID, mined.View.AuthorityOrigin, true)
		selected := mined.Projection.Items[0]
		live := request("manifest_portrait", selected.Sequence).Portrait
		if live.View.ViewID != mined.View.ViewID || live.ID != selected.ID || live.PortraitDigest != selected.PortraitDigest {
			t.Fatal("live portrait mismatch")
		}
		stopped := *request("stop", 0).Snapshot
		if stopped.State != "stopped" {
			t.Fatal("Stop did not join")
		}
		request("refresh_manifest_prospects", 0)
		refreshed := wait(func(s pilot.ManifestSnapshot) bool {
			return s.Projection.State == "ready" && s.Projection.CompleteRecent
		})
		winning := false
		for _, item := range refreshed.Projection.Items {
			receipt := request("manifest_receipt", item.Sequence).ManifestReceipt
			portrait := request("manifest_portrait", item.Sequence).Portrait
			if receipt.View != *refreshed.View || portrait.View != *refreshed.View || receipt.Record.Verified.ProspectID != item.ID || portrait.ID != item.ID {
				t.Fatal("mixed view")
			}
			if item.EligibilityMode == "canonical-winning-block" {
				winning = true
			}
			rows = append(rows, saved{fixture, *receipt, *portrait})
		}
		if !winning {
			t.Fatal("normal run did not include a canonical winner")
		}
		continued := false
		for _, winner := range rows[firstRow:] {
			if winner.Receipt.Record.Eligibility.Source != "canonical-winning-block" {
				continue
			}
			for _, later := range rows[firstRow:] {
				if later.Receipt.Record.Sequence > winner.Receipt.Record.Sequence && later.Receipt.Record.Context.ParentHeight > winner.Receipt.Record.Context.ParentHeight {
					continued = true
				}
			}
		}
		if !continued {
			t.Fatal("no later job after canonical win")
		}
		t.Log("live portraits and canonical continuation", fixture, len(rows)-firstRow)
		original := rows[len(rows)-1]
		exportPath := filepath.Join(root, fixture+"-public-receipt.json")
		if _, e = b.Request(ctx, "manifest_receipt", original.Receipt.Record.Sequence); e != nil {
			t.Fatal(e)
		}
		if _, e = b.ExportManifest(ctx, original.Receipt.Record.Sequence, original.Receipt.View.ViewID, original.Receipt.Record.Verified.ProspectID, exportPath); e != nil {
			t.Fatal("native selected export", e)
		}
		var exported registry.ManifestRecord
		raw, e := os.ReadFile(exportPath)
		if e != nil || len(raw) > registry.MaxRecordSize+1 || json.Unmarshal(raw, &exported) != nil {
			t.Fatal("public export unreadable", e)
		}
		if !reflect.DeepEqual(exported, original.Receipt.Record) {
			t.Fatal("public export differs from selected original receipt")
		}
		canonical, e := json.Marshal(original.Receipt.Record)
		if e != nil || !bytes.Equal(raw, append(canonical, '\n')) {
			t.Fatal("public export is not exact canonical selected receipt")
		}

		request("logout", 0)
		wait(func(s pilot.ManifestSnapshot) bool { return s.Account.State == "signed_out" })
		if _, e = b.Request(ctx, "manifest_portrait", original.Portrait.Sequence); e == nil {
			t.Fatal("signed-out disclosure")
		}
		if _, e = b.RequestFixture(ctx, fixture); e != nil {
			t.Fatal(e)
		}
		reopened := wait(func(s pilot.ManifestSnapshot) bool { return s.Account.State == "signed_in" })
		if reopened.View.ViewID == original.Receipt.View.ViewID || reopened.View.AuthorityPin != original.Receipt.View.AuthorityPin || reopened.Attempts != stopped.Attempts || reopened.AcceptedCount != stopped.AcceptedCount || !reopened.Projection.CompleteRecent {
			t.Fatal("reopen changed identity or mined")
		}
		again := request("manifest_receipt", original.Receipt.Record.Sequence).ManifestReceipt
		image := request("manifest_portrait", original.Portrait.Sequence).Portrait
		if !reflect.DeepEqual(again.Record, original.Receipt.Record) || image.SVG != original.Portrait.SVG {
			t.Fatal("cold reopen changed receipt or image")
		}
		request("reset_fixture", 0)
		held := wait(func(s pilot.ManifestSnapshot) bool {
			return s.Account.State == "signed_in" && s.Account.Held && s.Account.Generation > reopened.Account.Generation
		})
		t.Log("same-account held reset recovery", fixture)
		if !held.Account.Held || held.Account.CanExport || held.View.AuthorityPin != reopened.View.AuthorityPin {
			t.Fatal("reset lost hold or identity")
		}
		deniedPath := filepath.Join(root, fixture+"-denied-recovery.json")
		if _, e = b.RequestPath(ctx, "export_recovery", deniedPath); e == nil {
			t.Fatal("held recovery exported")
		}
		if _, e = os.Lstat(deniedPath); !os.IsNotExist(e) {
			t.Fatal("held recovery denial left a destination file", e)
		}
		inventory.capture(t, fixture, "reset-prepared", held.Account.ID, held.View.AuthorityOrigin, false)
		request("start", 0)
		heldMined := wait(func(s pilot.ManifestSnapshot) bool {
			return s.AcceptedCount > held.AcceptedCount && s.State == "running"
		})
		inventory.capture(t, fixture, "held-mining", heldMined.Account.ID, heldMined.View.AuthorityOrigin, true)
		request("stop", 0)
		request("logout", 0)
		wait(func(s pilot.ManifestSnapshot) bool { return s.Account.State == "signed_out" })
	}
	if e = b.Close(); e != nil {
		t.Fatal("helper did not close", e)
	}
	cleanupChecked = true
	inventory.checkClosed(t)
	if len(inventory.nodes) < 8 {
		t.Error("missing observed primary/peer process evidence across both mining phases and fixtures")
	}
	if b.cmd.ProcessState == nil || !b.cmd.ProcessState.Exited() || b.cmd.ProcessState.ExitCode() != 0 {
		t.Error("helper exit was not clean")
	}
	if t.Failed() {
		t.Fatal("refusing success evidence after failure")
	}
	raw, e := json.MarshalIndent(map[string]any{"status": "passed", "fixture_only": true, "public_chain_mined": false, "helper_exit": b.cmd.ProcessState.ExitCode(), "helper_pid": b.cmd.Process.Pid, "cleanup": inventory.evidence(), "exported_records_equal_selected": true, "denied_recovery_files_absent": true, "rows": rows}, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "public-result.json"), append(raw, 10), 0600); e != nil {
		t.Fatal(e)
	}
}

// This inventory reads only the two fixture account directories disclosed by
// the authenticated helper. It never logs node config bytes or RPC credentials,
// never signals a process, and never treats unrelated processes as test-owned.
type manifestObservedProcess struct {
	PID                int      `json:"pid"`
	ParentPID          int      `json:"parent_pid"`
	Fixture            string   `json:"fixture"`
	Phase              string   `json:"phase"`
	Node               string   `json:"node"`
	Config             string   `json:"config_relative"`
	Listeners          []string `json:"listeners"`
	OwnedCommandAbsent bool     `json:"owned_command_absent_after_close"`
}
type manifestProcessInventory struct {
	root, binary string
	helperPID    int
	nodes        []manifestObservedProcess
	listeners    map[string]bool
	configs      map[string]bool
	closed       bool
}

func newManifestProcessInventory(root, binary string, pid int) *manifestProcessInventory {
	return &manifestProcessInventory{root: root, binary: binary, helperPID: pid, listeners: map[string]bool{}, configs: map[string]bool{}}
}
func manifestReadTestConfig(t *testing.T, path string) []string {
	t.Helper()
	f, e := os.Open(path)
	if e != nil {
		t.Fatal("owned node config unavailable")
	}
	raw, e := io.ReadAll(io.LimitReader(f, 8193))
	closeErr := f.Close()
	if e != nil || closeErr != nil || len(raw) > 8192 {
		t.Fatal("owned node config bound/read failed")
	}
	listeners := []string{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || (key != "rpclisten" && key != "listen") {
			continue
		}
		host, port, e := net.SplitHostPort(value)
		n, parseErr := strconv.Atoi(port)
		if e != nil || parseErr != nil || host != "127.0.0.1" || n < 1 || n > 65535 {
			t.Fatal("owned listener is not explicit loopback")
		}
		listeners = append(listeners, value)
	}
	if len(listeners) < 1 || len(listeners) > 2 {
		t.Fatal("missing/oversized owned listener metadata")
	}
	return listeners
}
func manifestTestProcessQuery(t *testing.T, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, e := exec.CommandContext(ctx, args[0], args[1:]...).Output()
	if e != nil {
		var exit *exec.ExitError
		if errors.As(e, &exit) && exit.ExitCode() == 1 {
			return nil
		}
		t.Fatal("bounded owned-process inspection failed")
	}
	if len(raw) > 65536 {
		t.Fatal("owned-process inspection exceeded bound")
	}
	return raw
}
func (i *manifestProcessInventory) capture(t *testing.T, fixture, phase, accountID, origin string, running bool) {
	t.Helper()
	if !publicHex.MatchString(accountID) {
		t.Fatal("invalid owned account path")
	}
	authority, e := url.Parse(origin)
	if e != nil || authority.Scheme != "http" || authority.Hostname() != "127.0.0.1" || authority.Port() == "" {
		t.Fatal("invalid owned authority origin")
	}
	i.listeners[authority.Host] = true
	configs := map[string]string{}
	ports := map[string][]string{}
	for _, name := range []string{"primary", "peer"} {
		path := filepath.Join(i.root, "gyms", accountID, name+".conf")
		listeners := manifestReadTestConfig(t, path)
		configs[path] = name
		ports[path] = listeners
		i.configs[path] = true
		for _, address := range listeners {
			i.listeners[address] = true
		}
	}
	if !running {
		return
	}
	childPIDs := strings.Fields(string(manifestTestProcessQuery(t, "pgrep", "-P", strconv.Itoa(i.helperPID))))
	found := map[string]bool{}
	for _, field := range childPIDs {
		pid, e := strconv.Atoi(field)
		if e != nil || pid <= 0 {
			t.Fatal("invalid child PID inspection")
		}
		raw := strings.TrimSpace(string(manifestTestProcessQuery(t, "ps", "-ww", "-p", field, "-o", "ppid=", "-o", "command=")))
		fields := strings.Fields(raw)
		if len(fields) < 2 {
			continue
		}
		parent, e := strconv.Atoi(fields[0])
		if e != nil || parent != i.helperPID {
			continue
		}
		command := strings.TrimSpace(strings.TrimPrefix(raw, fields[0]))
		for path, name := range configs {
			if command != i.binary+" --configfile="+path {
				continue
			}
			if found[path] {
				t.Fatal("duplicate owned node processes for one config")
			}
			found[path] = true
			relative, e := filepath.Rel(i.root, path)
			if e != nil {
				t.Fatal("owned config path")
			}
			i.nodes = append(i.nodes, manifestObservedProcess{PID: pid, ParentPID: parent, Fixture: fixture, Phase: phase, Node: name, Config: relative, Listeners: append([]string{}, ports[path]...)})
		}
	}
	if len(found) != 2 {
		t.Fatalf("expected both owned helper child node processes; observed %d", len(found))
	}
}
func (i *manifestProcessInventory) checkClosed(t *testing.T) {
	t.Helper()
	allClosed := true
	for address := range i.listeners {
		connection, e := net.DialTimeout("tcp4", address, 200*time.Millisecond)
		if e == nil {
			connection.Close()
			allClosed = false
			t.Errorf("owned fixture listener survived Close at %s", address)
		}
		if e != nil && !errors.Is(e, syscall.ECONNREFUSED) {
			allClosed = false
			t.Errorf("owned listener closure was not observable at %s", address)
		}
	}
	// Match exact owned config arguments, irrespective of parent PID. This also
	// finds a reparented leftover, without selecting or affecting other nodes.
	absent := map[string]bool{}
	for path := range i.configs {
		pattern := "(^|[[:space:]])--configfile=" + regexp.QuoteMeta(path) + "($|[[:space:]])"
		matches := strings.Fields(string(manifestTestProcessQuery(t, "pgrep", "-f", pattern)))
		absent[path] = len(matches) == 0
		if len(matches) != 0 {
			allClosed = false
			t.Errorf("owned node config still has %d matching processes", len(matches))
		}
	}
	for n := range i.nodes {
		path := filepath.Join(i.root, i.nodes[n].Config)
		i.nodes[n].OwnedCommandAbsent = absent[path]
	}
	i.closed = allClosed
	t.Logf("verified %d captured private listeners and %d owned config commands after helper Close", len(i.listeners), len(i.configs))
}
func (i *manifestProcessInventory) evidence() map[string]any {
	addresses := make([]string, 0, len(i.listeners))
	for a := range i.listeners {
		addresses = append(addresses, a)
	}
	sort.Strings(addresses)
	return map[string]any{"observed_owned_nodes": i.nodes, "captured_listener_addresses": addresses, "all_captured_listeners_closed": i.closed, "all_owned_config_commands_absent": i.closed, "owned_config_count": len(i.configs), "process_exit_code_source": "helper exit and successful Close; direct child exit codes are not available to the bridge test", "process_observation_scope": "both explicit mining phases per fixture; Prepare-only processes are not individually observed", "read_only_process_inspection": true}
}
