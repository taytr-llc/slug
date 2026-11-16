// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// slugcheckpoint creates and verifies signed SLUG checkpoints offline. It does
// not connect to a node or decide whether a block is eligible for finality.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
)

const usage = "usage: slugcheckpoint {keygen|sign|verify} [options]"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "slugcheckpoint:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "keygen":
		return keygen(args[1:], stdout, stderr)
	case "sign":
		return sign(args[1:], stdout, stderr)
	case "verify":
		return verify(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		_, err := fmt.Fprintln(stdout, usage)
		return err
	default:
		return errors.New(usage)
	}
}

func commandFlags(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	return nil
}

// writeExclusive never replaces an existing file, including a symbolic link.
func writeExclusive(path string, contents []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		file.Close()
		if !complete {
			os.Remove(path)
		}
	}()
	if _, err := file.Write(contents); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func keygen(args []string, stdout, stderr io.Writer) error {
	flags := commandFlags("keygen", stderr)
	output := flags.String("out", "", "New private seed file (32 raw bytes; never overwritten)")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if *output == "" {
		return errors.New("--out is required")
	}
	seed := make([]byte, ed25519.SeedSize)
	defer clear(seed)
	if _, err := rand.Read(seed); err != nil {
		return err
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	defer clear(privateKey)
	if err := writeExclusive(*output, seed, 0600); err != nil {
		return fmt.Errorf("create private seed file: %w", err)
	}
	_, err := fmt.Fprintln(stdout, hex.EncodeToString(privateKey.Public().(ed25519.PublicKey)))
	return err
}

func networkParams(name string) (*chaincfg.Params, error) {
	switch name {
	case "mainnet":
		return &chaincfg.MainNetParams, nil
	case "regtest":
		return &chaincfg.RegressionNetParams, nil
	case "simnet":
		return &chaincfg.SimNetParams, nil
	default:
		return nil, errors.New("--network must be mainnet, regtest, or simnet")
	}
}

func parseHash(value string) (*chainhash.Hash, error) {
	if len(value) != chainhash.HashSize*2 {
		return nil, errors.New("hash must contain exactly 64 hexadecimal characters")
	}
	return chainhash.NewHashFromStr(value)
}

func readBoundedFile(path string, limit int64, private bool) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("input must be a regular file")
	}
	if private && runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("private seed file must not grant group or other permissions (use chmod 600)")
	}
	contents, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > limit {
		return nil, errors.New("input file exceeds size limit")
	}
	return contents, nil
}

func sign(args []string, stdout, stderr io.Writer) error {
	flags := commandFlags("sign", stderr)
	keyFile := flags.String("key", "", "Private seed file created by keygen")
	network := flags.String("network", "mainnet", "Network: mainnet, regtest, or simnet")
	genesis := flags.String("genesis", "", "Optional expected genesis hash; must match the selected network")
	height := flags.Int64("height", 0, "Positive checkpoint height")
	hash := flags.String("hash", "", "Checkpoint block hash (64 hexadecimal characters)")
	output := flags.String("out", "", "New signed checkpoint JSON file (never overwritten)")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if *keyFile == "" || *output == "" {
		return errors.New("--key and --out are required")
	}
	if *height <= 0 || *height > 1<<31-1 {
		return errors.New("--height must be between 1 and 2147483647")
	}
	params, err := networkParams(*network)
	if err != nil {
		return err
	}
	if *genesis != "" {
		parsedGenesis, err := parseHash(*genesis)
		if err != nil || !parsedGenesis.IsEqual(params.GenesisHash) {
			return errors.New("--genesis does not match the selected network")
		}
	}
	blockHash, err := parseHash(*hash)
	if err != nil {
		return fmt.Errorf("--hash: %w", err)
	}
	checkpoint := chaincfg.SignedCheckpoint{
		Version:     chaincfg.SignedCheckpointVersion,
		Network:     params.Net,
		GenesisHash: *params.GenesisHash,
		Height:      int32(*height),
		Hash:        *blockHash,
	}
	message, err := checkpoint.SigningBytes()
	if err != nil {
		return err
	}
	seed, err := readBoundedFile(*keyFile, ed25519.SeedSize, true)
	if err != nil {
		return fmt.Errorf("read private seed file: %w", err)
	}
	defer clear(seed)
	if len(seed) != ed25519.SeedSize {
		return errors.New("private seed file must contain exactly 32 raw bytes")
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	defer clear(privateKey)
	checkpoint.Signature = ed25519.Sign(privateKey, message)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if err := checkpoint.Verify(params, publicKey); err != nil {
		return fmt.Errorf("verify generated signature: %w", err)
	}
	encoded, err := json.MarshalIndent(&checkpoint, "", "  ")
	if err != nil {
		return err
	}
	if err := writeExclusive(*output, append(encoded, '\n'), 0644); err != nil {
		return fmt.Errorf("create signed checkpoint: %w", err)
	}
	_, err = fmt.Fprintf(stdout, "Signed height %d on %s with public key %x\n",
		checkpoint.Height, *network, publicKey)
	return err
}

func verify(args []string, stdout, stderr io.Writer) error {
	flags := commandFlags("verify", stderr)
	key := flags.String("pubkey", "", "Trusted Ed25519 public key (64 hexadecimal characters)")
	network := flags.String("network", "mainnet", "Network: mainnet, regtest, or simnet")
	input := flags.String("file", "", "Signed checkpoint JSON file")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if *input == "" {
		return errors.New("--file is required")
	}
	publicKey, err := hex.DecodeString(*key)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("--pubkey must contain exactly 64 hexadecimal characters")
	}
	params, err := networkParams(*network)
	if err != nil {
		return err
	}
	encoded, err := readBoundedFile(*input, chaincfg.MaxSignedCheckpointSize, false)
	if err != nil {
		return err
	}
	var checkpoint chaincfg.SignedCheckpoint
	if err := json.Unmarshal(encoded, &checkpoint); err != nil {
		return fmt.Errorf("decode signed checkpoint: %w", err)
	}
	if err := checkpoint.Verify(params, ed25519.PublicKey(publicKey)); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Valid signature for height %d, block %s on %s\n",
		checkpoint.Height, checkpoint.Hash, *network)
	return err
}
