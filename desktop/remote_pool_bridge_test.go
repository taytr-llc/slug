//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
)

type nativeRemoteReference struct {
	Configs []struct {
		Name     string                 `json:"name"`
		Config   pilot.RemotePoolConfig `json:"config"`
		ConfigID string                 `json:"config_id"`
	} `json:"configs"`
	Positives []struct {
		Name     string                   `json:"name"`
		Response pilot.RemotePoolResponse `json:"response"`
	} `json:"positives"`
}

func nativeRemoteReferenceRead(path string) (out nativeRemoteReference, e error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return out, e
	}
	h := sha256.Sum256(raw)
	if len(raw) != 490221 || hex.EncodeToString(h[:]) != "001430345f09c1fc8e1c924db18564250df6b5f32e21a2cddcfbc8d6be83a865" {
		return out, errors.New("public protocol fixture changed")
	}
	e = json.Unmarshal(raw, &out)
	return
}

// Only a Go test binary contains this entry. LaunchRemotePool's exact argv is
// retained; the child speaks public corpus frames and owns no wallet/network.
// This is native-facade/process-fence coverage, not actual controller or UI proof.
func init() {
	if os.Getenv("SLUG_REMOTE_NATIVE_TEST_FIXTURE") != "1" || len(os.Args) < 2 || os.Args[1] != "--candidate-remote-pool" {
		return
	}
	signal.Ignore(syscall.SIGPIPE)
	ref, e := nativeRemoteReferenceRead(os.Getenv("SLUG_REMOTE_NATIVE_TEST_REFERENCE"))
	if e != nil {
		os.Exit(71)
	}
	selection, e := parseNativeRemoteArgs(os.Args[1:])
	if e != nil {
		os.Exit(72)
	}
	active := false
	input := bufio.NewScanner(os.Stdin)
	input.Buffer(make([]byte, 2048), pilot.MaxRemotePoolRequestSize+1)
	for input.Scan() {
		var request pilot.RemotePoolRequest
		if json.Unmarshal(input.Bytes(), &request) != nil {
			os.Exit(73)
		}
		name := "zero256-default-" + request.Command
		if request.Command == "snapshot" && active {
			name = "zero256-default-snapshot-original"
		}
		var response *pilot.RemotePoolResponse
		for _, p := range ref.Positives {
			if p.Name == name {
				v := p.Response
				response = &v
				break
			}
		}
		if response == nil {
			os.Exit(74)
		}
		response.ID, response.Command, response.ConfigID, response.ViewID = request.ID, request.Command, request.ConfigID, request.ViewID
		if request.Command == "login_fixture" {
			active = true
		}
		if request.Command == "logout" {
			active = false
		}
		if request.Command == "start" {
			switch os.Getenv("SLUG_REMOTE_NATIVE_TEST_MODE") {
			case "lost":
				if os.WriteFile(selection.ConfigPath+".called", []byte("start received\n"), 0600) != nil {
					os.Exit(79)
				}
				_, _ = io.Copy(io.Discard, os.Stdin)
				os.Exit(0)
			case "replace":
				raw, e := os.ReadFile(selection.ConfigPath)
				if e != nil {
					os.Exit(75)
				}
				if os.Rename(selection.ConfigPath, selection.ConfigPath+".retained") != nil || os.WriteFile(selection.ConfigPath, raw, 0600) != nil {
					os.Exit(76)
				}
			case "unknown":
				response.OK, response.Result, response.Error = false, nil, "outcome_unknown"
			}
		}
		raw, e := json.Marshal(response)
		if e != nil {
			os.Exit(77)
		}
		if _, e = os.Stdout.Write(append(raw, '\n')); e != nil {
			os.Exit(0)
		}
	}
	if input.Err() != nil {
		os.Exit(78)
	}
	os.Exit(0)
}

func nativeRemoteTestOpen(t *testing.T, mode string) (*nativeRemoteOwner, string) {
	t.Helper()
	path, e := filepath.Abs("pilot/testdata/remote-pool-protocol/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	ref, e := nativeRemoteReferenceRead(path)
	if e != nil {
		t.Fatal(e)
	}
	var config pilot.RemotePoolConfig
	for _, c := range ref.Configs {
		if c.Name == "zero256-default" {
			config = c.Config
		}
	}
	raw, e := pilot.MarshalRemotePoolConfig(config)
	if e != nil {
		t.Fatal(e)
	}
	d, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(d, 0700); e != nil {
		t.Fatal(e)
	}
	selection, e := parseNativeRemoteArgs(nativeRemoteTestArgs(d))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(selection.ConfigPath, raw, 0600); e != nil {
		t.Fatal(e)
	}
	// RPC/bearer/root paths remain absent; native must never open them.
	t.Setenv("SLUG_REMOTE_NATIVE_TEST_FIXTURE", "1")
	t.Setenv("SLUG_REMOTE_NATIVE_TEST_REFERENCE", path)
	t.Setenv("SLUG_REMOTE_NATIVE_TEST_MODE", mode)
	helper, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	life, cancel := context.WithCancel(context.Background())
	o, e := openNativeRemote(life, helper, selection)
	if o != nil {
		t.Cleanup(func() {
			cancel()
			if e := o.close(); e != nil {
				t.Error(e)
			}
		})
	} else {
		cancel()
	}
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{selection.NodeRPCPath, selection.PoolCredentialPath, selection.AdmissionCredentialPath, selection.ExistingRoot, selection.AttachmentRoot} {
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			t.Fatal("native read/created private or account input")
		}
	}
	return o, selection.ConfigPath
}
func TestNativeRemoteConcreteBridgeOriginalViewsAndMenu(t *testing.T) {
	o, _ := nativeRemoteTestOpen(t, "normal")
	g := &RemoteGym{owner: o}
	if _, e := g.Snapshot(); e != nil {
		t.Fatal(e)
	}
	if _, e := g.LoginFixture("incrementing256"); e == nil {
		t.Fatal("foreign fixture admitted")
	}
	s, e := g.LoginFixture("zero256")
	if e != nil || s.View == nil {
		t.Fatal("login", e)
	}
	view := s.View.ViewID
	menu := g.menuAction("start")
	if _, e = g.Start(strings.Repeat("e", 64)); e == nil {
		t.Fatal("foreign view dispatched")
	}
	if e = menu(); e != nil {
		t.Fatal("original menu", e)
	}
	if s, e = g.Stop(view); e != nil || s.Remote.State != "stopped" {
		t.Fatal("stop", e)
	}
	if s, e = g.Logout(view); e != nil || s.View != nil || s.Remote != nil {
		t.Fatal("logout", e)
	}
	if e = menu(); e == nil {
		t.Fatal("retired menu rebound to later state")
	}
	if _, e = g.Start(view); e == nil {
		t.Fatal("retired view reused")
	}
	if _, e = g.Snapshot(); e != nil {
		t.Fatal("stale caller retired a healthy original helper", e)
	}
}
func TestNativeRemotePostMutationConfigChangeIsUnknown(t *testing.T) {
	o, _ := nativeRemoteTestOpen(t, "replace")
	g := &RemoteGym{owner: o}
	s, e := g.LoginFixture("zero256")
	if e != nil {
		t.Fatal(e)
	}
	s, e = g.Start(s.View.ViewID)
	if !errors.Is(e, bridge.ErrRemotePoolOutcomeUnknown) || s.View != nil || !o.closing {
		t.Fatal("post-action replacement disclosed success", e)
	}
	if e = o.close(); e != nil {
		t.Fatal("helper did not join", e)
	}
}
func TestNativeRemoteCloseCancelsActiveOriginalHelper(t *testing.T) {
	o, configPath := nativeRemoteTestOpen(t, "lost")
	g := &RemoteGym{owner: o}
	s, e := g.LoginFixture("zero256")
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := g.Start(s.View.ViewID); done <- e }()
	joined := false
	defer func() {
		_ = o.close()
		if !joined {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("test call did not join")
			}
		}
	}()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	// The bounded public fixture marker proves this exact Start reached the
	// helper before cancellation. No arbitrary scheduling sleep or new deadline.
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if _, e := os.Stat(configPath + ".called"); e == nil {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("call was not received")
		case <-tick.C:
		}
	}
	if e = o.close(); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		joined = true
		if e == nil {
			t.Fatal("canceled mutation succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call not joined")
	}
	if !o.closing || !o.input.closeDone {
		t.Fatal("cleanup did not retain/join exact owners")
	}
}
