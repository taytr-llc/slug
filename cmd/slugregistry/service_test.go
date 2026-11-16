// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestServiceCommands(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "prospectjob", "testdata", "jobs_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var refs []struct {
		Job           prospectjob.Job       `json:"job"`
		Proof         prospect.Proof        `json:"proof"`
		Authorization gymauth.Authorization `json:"authorization"`
		Verified      prospect.Verified     `json:"expected_verified"`
	}
	if err := json.Unmarshal(raw, &refs); err != nil {
		t.Fatal(err)
	}
	f := refs[0]
	receipt := registry.Record{Version: 3, Sequence: 1, Context: f.Job.Context, Proof: f.Proof, Verified: f.Verified, Authorization: &f.Authorization, Job: &f.Job,
		Birth: &registry.Birth{RuleVersion: 1, BornAtMS: f.Job.IssuedAtMS, AcceptedAtMS: f.Job.IssuedAtMS + 1}}
	result := prospectservice.AdmissionResult{Version: 1, ContextSource: "historical-receipt", Authentication: "gym-key-control", BirthRule: "job-issuance", Record: receipt}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/jobs" {
			_ = json.NewEncoder(w).Encode(f.Job)
		} else {
			_ = json.NewEncoder(w).Encode(result)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	config, policy, job, proof, auth := filepath.Join(dir, "service.json"), filepath.Join(dir, "policy.json"), filepath.Join(dir, "job.json"), filepath.Join(dir, "proof.json"), filepath.Join(dir, "auth.json")
	writeTestJSON(t, config, prospectservice.ClientConfig{Version: 1, URL: server.URL, Token: strings.Repeat("42", 32)})
	writeTestJSON(t, policy, prospectnodePolicy{1, f.Job.Context.Network, f.Job.Context.Genesis, 1, f.Job.Context.ProspectTarget})
	writeTestJSON(t, job, f.Job)
	writeTestJSON(t, proof, f.Proof)
	writeTestJSON(t, auth, f.Authorization)
	for _, args := range [][]string{{"issue-job-service", "--service-config", config, "--policy", policy, "--public-key", f.Job.PublicKey},
		{"record-job-service", "--service-config", config, "--policy", policy, "--job", job, "--proof", proof, "--authorization", auth}} {
		var stdout, stderr bytes.Buffer
		err := run(args, &stdout, &stderr)
		if wire.TestPoW {
			if err == nil || stdout.Len() != 0 {
				t.Fatal("cheap profile used production authority")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if args[0] == "issue-job-service" {
			var got prospectjob.Job
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || got != f.Job {
				t.Fatal("wrong service job")
			}
		} else {
			var got prospectservice.AdmissionResult
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !reflect.DeepEqual(got, result) {
				t.Fatal("wrong service receipt")
			}
		}
	}
	for _, command := range []string{"issue-job-service", "record-job-service"} {
		for _, args := range [][]string{{command}, {command, "--db", dir}, {command, "--token", strings.Repeat("42", 32)}, {command, "--private-key", "unsupported"}} {
			var stdout, stderr bytes.Buffer
			if err := run(args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
				t.Fatal("invalid service arguments accepted")
			}
		}
		var stdout, stderr bytes.Buffer
		if err := run([]string{command, "-h"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) || !strings.Contains(stderr.String(), "usage: slugregistry "+command) {
			t.Fatal("missing service help")
		}
	}
}
