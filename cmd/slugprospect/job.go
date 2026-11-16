// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

func commitmentJob(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("commitment-job", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jobPath := flags.String("job", "", "Public issued-job JSON file (required)")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: slugprospect commitment-job [options]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *jobPath == "" {
		return errors.New("--job is required")
	}
	data, err := readBoundedFile(*jobPath, prospectjob.MaxJobSize)
	if err != nil {
		return fmt.Errorf("read job: %w", err)
	}
	job, err := prospectjob.ParseJob(data)
	if err != nil {
		return err
	}
	script, err := prospectjob.CommitmentScript(job)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
		ProofVersion uint32           `json:"proof_version"`
		JobID        string           `json:"job_id"`
		Context      prospect.Context `json:"context"`
		GymID        string           `json:"gym_id"`
		ScriptHex    string           `json:"script_hex"`
	}{prospect.Version2, job.ID, job.Context, job.GymID, hex.EncodeToString(script)})
}
