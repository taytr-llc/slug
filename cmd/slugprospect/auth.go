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
	"strings"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

func readAuthorization(path string) (gymauth.Authorization, error) {
	data, err := readBoundedFile(path, gymauth.MaxAuthorizationSize)
	if err != nil {
		return gymauth.Authorization{}, fmt.Errorf("read authorization: %w", err)
	}
	return gymauth.ParseAuthorization(data)
}

func authorizationCommand(command string, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	contextPath := flags.String("context", "", "Trusted context JSON file (required)")
	var publicKey, proofPath, authorizationPath *string
	if command != "verify-auth" {
		publicKey = flags.String("public-key", "", "Compressed secp256k1 public key, 66 hex characters (required)")
	} else {
		authorizationPath = flags.String("authorization", "", "Public gym-key authorization JSON file (required)")
	}
	if command != "gym-id" {
		proofPath = flags.String("proof", "", "Untrusted proof JSON file (required)")
	}
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: slugprospect %s [options]\n", command)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *contextPath == "" {
		return errors.New("--context is required")
	}
	if publicKey != nil && *publicKey == "" {
		return errors.New("--public-key is required")
	}
	if proofPath != nil && *proofPath == "" {
		return errors.New("--proof is required")
	}
	if authorizationPath != nil && *authorizationPath == "" {
		return errors.New("--authorization is required")
	}
	data, err := readBoundedFile(*contextPath, prospect.MaxContextSize)
	if err != nil {
		return fmt.Errorf("read context: %w", err)
	}
	resolved, err := prospect.ParseContext(data)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if command == "gym-id" {
		gymID, err := gymauth.GymID(resolved, *publicKey)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Version uint32 `json:"version"`
			GymID   string `json:"gym_id"`
		}{gymauth.Version1, gymID})
	}
	data, err = readBoundedFile(*proofPath, prospect.MaxProofSize)
	if err != nil {
		return fmt.Errorf("read proof: %w", err)
	}
	proof, err := prospect.ParseProof(data)
	if err != nil {
		return err
	}
	if command == "auth-payload" {
		digest, err := gymauth.AdmissionDigest(resolved, proof, *publicKey)
		if err != nil {
			return err
		}
		gymID, err := gymauth.GymID(resolved, *publicKey)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Version   uint32 `json:"version"`
			Action    string `json:"action"`
			GymID     string `json:"gym_id"`
			PublicKey string `json:"public_key"`
			Digest    string `json:"digest"`
		}{gymauth.Version1, "prospect-admission", gymID, strings.ToLower(*publicKey), hex.EncodeToString(digest[:])})
	}
	authorization, err := readAuthorization(*authorizationPath)
	if err != nil {
		return err
	}
	if err := gymauth.VerifyAuthorization(resolved, proof, authorization); err != nil {
		return err
	}
	verified, err := prospect.Verify(resolved, proof)
	if err != nil {
		return err
	}
	return encoder.Encode(struct {
		Authentication string            `json:"authentication"`
		Verified       prospect.Verified `json:"verified"`
	}{"gym-key-control", verified})
}
