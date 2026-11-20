//go:build ignore

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This standalone oracle deliberately imports no SLUG package. Run it with
// go run argon_oracle.go HEADER_HEX [HEADER_HEX ...]. Each fixed 80-byte input
// receives exactly one Argon2id computation with literal production parameters.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

type result struct {
	Header   string `json:"header"`
	Digest   string `json:"digest"`
	WorkHash string `json:"work_hash"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if argon2.Version != 19 {
		return fmt.Errorf("unexpected Argon2 version")
	}
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: go run argon_oracle.go HEADER_HEX [HEADER_HEX ...]")
	}
	results := make([]result, 0, len(os.Args)-1)
	for _, encoded := range os.Args[1:] {
		if len(encoded) != 160 {
			return fmt.Errorf("header must have exactly 160 hexadecimal characters")
		}
		header, err := hex.DecodeString(encoded)
		if err != nil {
			return fmt.Errorf("invalid header: %w", err)
		}
		digest := argon2.IDKey(header, header, 1, 256*1024, 1, 32)
		var display [32]byte
		for i := range display {
			display[i] = digest[len(digest)-1-i]
		}
		results = append(results, result{
			Header:   hex.EncodeToString(header),
			Digest:   hex.EncodeToString(digest),
			WorkHash: hex.EncodeToString(display[:]),
		})
	}
	return json.NewEncoder(os.Stdout).Encode(results)
}
