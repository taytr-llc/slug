//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
)

type remoteBridgeReference struct {
	Configs []struct {
		Name   string                 `json:"name"`
		Config pilot.RemotePoolConfig `json:"config"`
	} `json:"configs"`
	Positives []struct {
		Name     string                   `json:"name"`
		Response pilot.RemotePoolResponse `json:"response"`
	} `json:"positives"`
}

func remoteBridgeReadReference(path string) (remoteBridgeReference, error) {
	var out remoteBridgeReference
	raw, e := os.ReadFile(path)
	if e != nil {
		return out, e
	}
	sum := sha256.Sum256(raw)
	if len(raw) != 490221 || hex.EncodeToString(sum[:]) != "001430345f09c1fc8e1c924db18564250df6b5f32e21a2cddcfbc8d6be83a865" {
		return out, ErrUnavailable
	}
	e = json.Unmarshal(raw, &out)
	return out, e
}

func remoteBridgeReferenceFor(t *testing.T, name string) (pilot.RemotePoolConfig, string) {
	t.Helper()
	path, e := filepath.Abs("../../pilot/testdata/remote-pool-protocol/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	r, e := remoteBridgeReadReference(path)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range r.Configs {
		if c.Name == name {
			return c.Config, path
		}
	}
	t.Fatal("missing independent public config")
	return pilot.RemotePoolConfig{}, ""
}

func remoteBridgeFixtureCommand(t *testing.T, config, mode string) (*exec.Cmd, pilot.RemotePoolConfig) {
	t.Helper()
	c, path := remoteBridgeReferenceFor(t, config)
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(executable, "-test.run=^TestRemotePoolBridgeHelperProcess$")
	cmd.Env = append(os.Environ(), "SLUG_REMOTE_BRIDGE_FIXTURE=1", "SLUG_REMOTE_BRIDGE_REFERENCE="+path,
		"SLUG_REMOTE_BRIDGE_CONFIG="+config, "SLUG_REMOTE_BRIDGE_MODE="+mode)
	return cmd, c
}

// This is a synthetic pipe process, not a wallet/controller/worker acceptance
// claim. Responses use independent corpus snapshots; only request correlations
// are copied at runtime. It never opens custody, network or account resources.
func TestRemotePoolBridgeHelperProcess(t *testing.T) {
	if os.Getenv("SLUG_REMOTE_BRIDGE_FIXTURE") != "1" {
		return
	}
	signal.Ignore(syscall.SIGPIPE)
	fixture, e := remoteBridgeReadReference(os.Getenv("SLUG_REMOTE_BRIDGE_REFERENCE"))
	if e != nil {
		os.Exit(20)
	}
	config, mode := os.Getenv("SLUG_REMOTE_BRIDGE_CONFIG"), os.Getenv("SLUG_REMOTE_BRIDGE_MODE")
	active := false
	input := bufio.NewScanner(os.Stdin)
	input.Buffer(make([]byte, 2048), pilot.MaxRemotePoolRequestSize+1)
	for input.Scan() {
		var request pilot.RemotePoolRequest
		if json.Unmarshal(input.Bytes(), &request) != nil {
			os.Exit(21)
		}
		name := config + "-" + request.Command
		if request.Command == "snapshot" && active {
			name = config + "-snapshot-original"
		}
		var response *pilot.RemotePoolResponse
		for _, p := range fixture.Positives {
			if p.Name == name {
				raw, _ := json.Marshal(p.Response)
				var v pilot.RemotePoolResponse
				_ = json.Unmarshal(raw, &v)
				response = &v
				break
			}
		}
		if response == nil {
			os.Exit(22)
		}
		response.ID, response.Command, response.ConfigID, response.ViewID = request.ID, request.Command, request.ConfigID, request.ViewID
		if request.Command == "login_fixture" {
			active = true
		}
		if request.Command == "logout" {
			active = false
		}
		if request.Command == "start" {
			switch mode {
			case "lost-reply":
				_, _ = io.Copy(io.Discard, os.Stdin)
				os.Exit(0)
			case "wrong-id":
				response.ID++
			case "wrong-config":
				response.ConfigID = strings.Repeat("f", 64)
			case "unknown", "cleanup", "busy":
				response.OK, response.Result, response.Error = false, nil, "outcome_unknown"
				if mode == "cleanup" {
					response.Error = "cleanup_incomplete"
				}
				if mode == "busy" {
					response.Error = "busy"
				}
			}
		}
		raw, e := json.Marshal(response)
		if e != nil {
			os.Exit(23)
		}
		line := append(raw, '\n')
		if request.Command == "start" {
			switch mode {
			case "missing-lf":
				_, _ = os.Stdout.Write(raw)
				_ = os.Stdout.Close()
				_, _ = io.Copy(io.Discard, os.Stdin)
				os.Exit(0)
			case "crlf":
				line = append(raw, '\r', '\n')
			case "oversize":
				line = append(bytes.Repeat([]byte{'x'}, pilot.MaxRemotePoolResponseSize+1), '\n')
			case "double":
				line = append(line, line...)
			}
		}
		if request.Command == "snapshot" && request.ID == 1 && mode == "unsolicited" {
			line = append(line, line...)
		}
		if _, e = os.Stdout.Write(line); e != nil {
			os.Exit(0)
		}
	}
	os.Exit(0)
}
