// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospect

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighter"
	"github.com/taytr-llc/slug/wire/v2"
)

const (
	// Version1 is the original prospect work proof and context format.
	Version1 uint32 = 1
	// Version2 binds prospect work to a separately validated issuance job.
	Version2 uint32 = 2

	// MaxContextSize bounds a serialized context, including whitespace.
	MaxContextSize = 4096

	// MaxProofSize bounds a serialized proof, including whitespace.
	MaxProofSize = 49152

	maxCoinbaseSize = 16384
	maxMerkleBranch = 20
	maxOutputs      = 256
	maxMoney        = int64(21000000 * 100000000)

	policyDomain = "SLUG prospect policy\x00v1\x00"
	gymDomain    = "SLUG prospect gym\x00v1\x00"
	jobGymDomain = "SLUG prospect job gym\x00v2\x00"
	idDomain     = "SLUG prospect id\x00v1\x00"
	marker       = "SLUGPR01"
	jobMarker    = "SLUGPR02"
	markerFamily = "SLUGPR"
)

// Context contains trusted policy and parent-chain assertions. Verify checks
// consistency with these assertions; it does not authenticate their source.
type Context struct {
	Version        uint32 `json:"version"`
	Network        uint32 `json:"network"`
	Genesis        string `json:"genesis"`
	DNAVersion     uint32 `json:"dna_version"`
	ProspectTarget string `json:"prospect_target"`
	ParentHash     string `json:"parent_hash"`
	ParentHeight   uint32 `json:"parent_height"`
	ExpectedBits   uint32 `json:"expected_bits"`
	MinTimestamp   uint32 `json:"min_timestamp"`
	MaxTimestamp   uint32 `json:"max_timestamp"`
}

// Proof contains a serialized header and an index-zero coinbase Merkle path.
// Strings are exact hexadecimal encodings; Merkle siblings use display order.
// Version 2 requires JobID, which binds the work to a job identifier without
// authenticating that job's issuance, expiry, clock or birth policy.
type Proof struct {
	Version      uint32   `json:"version"`
	GymID        string   `json:"gym_id"`
	Header       string   `json:"header"`
	Coinbase     string   `json:"coinbase"`
	MerkleBranch []string `json:"merkle_branch"`
	JobID        string   `json:"job_id,omitempty"`
}

// Verified records work meeting the supplied context. It is a value snapshot,
// not an ownership record, authenticated birth date, or full-block validation.
type Verified struct {
	Version      uint32      `json:"version"`
	Verification string      `json:"verification"`
	Network      uint32      `json:"network"`
	Genesis      string      `json:"genesis"`
	ProspectID   string      `json:"prospect_id"`
	PolicyID     string      `json:"policy_id"`
	GymID        string      `json:"gym_id"`
	WorkHash     string      `json:"work_hash"`
	ParentHash   string      `json:"parent_hash"`
	ParentHeight uint32      `json:"parent_height"`
	DNA          fighter.DNA `json:"dna"`
}

type parsedContext struct {
	genesis [32]byte
	target  [32]byte
	parent  [32]byte
	policy  [32]byte
}

func decode32(name, value string, nonzero bool) ([32]byte, error) {
	var result [32]byte
	if len(value) != 64 {
		return result, fmt.Errorf("%s must contain exactly 64 hexadecimal characters", name)
	}
	if _, err := hex.Decode(result[:], []byte(value)); err != nil {
		return result, fmt.Errorf("invalid %s: %w", name, err)
	}
	if nonzero && result == ([32]byte{}) {
		return result, fmt.Errorf("%s must be nonzero", name)
	}
	return result, nil
}

func parseContext(context Context) (parsedContext, error) {
	var parsed parsedContext
	if context.Version != Version1 {
		return parsed, fmt.Errorf("unsupported context version %d", context.Version)
	}
	if context.Network == 0 {
		return parsed, fmt.Errorf("network must be nonzero")
	}
	if context.DNAVersion != fighter.Version1 {
		return parsed, fmt.Errorf("unsupported DNA version %d", context.DNAVersion)
	}
	if context.ParentHeight > 2147483646 {
		return parsed, fmt.Errorf("parent height exceeds 2147483646")
	}
	if context.ExpectedBits == 0 {
		return parsed, fmt.Errorf("expected bits must be nonzero")
	}
	if context.MaxTimestamp < context.MinTimestamp {
		return parsed, fmt.Errorf("timestamp interval is inverted")
	}
	var err error
	parsed.genesis, err = decode32("genesis", context.Genesis, true)
	if err != nil {
		return parsed, err
	}
	parsed.target, err = decode32("prospect target", context.ProspectTarget, true)
	if err != nil {
		return parsed, err
	}
	parsed.parent, err = decode32("parent hash", context.ParentHash, true)
	if err != nil {
		return parsed, err
	}
	payload := append([]byte(policyDomain), le32(context.Network)...)
	payload = append(payload, parsed.genesis[:]...)
	payload = append(payload, le32(context.DNAVersion)...)
	payload = append(payload, parsed.target[:]...)
	parsed.policy = sha256.Sum256(payload)
	return parsed, nil
}

func le32(value uint32) []byte {
	var result [4]byte
	binary.LittleEndian.PutUint32(result[:], value)
	return result[:]
}

func commitmentScript(policy, gym [32]byte) []byte {
	payload := append([]byte(gymDomain), policy[:]...)
	payload = append(payload, gym[:]...)
	commitment := sha256.Sum256(payload)
	script := append([]byte{0x6a, 40}, marker...)
	return append(script, commitment[:]...)
}

// CommitmentScript returns the canonical zero-value output script to include
// in a mining template. It does not mine or authenticate the gym identifier.
func CommitmentScript(context Context, gymID string) ([]byte, error) {
	parsed, err := parseContext(context)
	if err != nil {
		return nil, err
	}
	gym, err := decode32("gym ID", gymID, true)
	if err != nil {
		return nil, err
	}
	return commitmentScript(parsed.policy, gym), nil
}

func jobCommitmentScript(policy, gym, job [32]byte) []byte {
	payload := append([]byte(jobGymDomain), policy[:]...)
	payload = append(payload, gym[:]...)
	payload = append(payload, job[:]...)
	commitment := sha256.Sum256(payload)
	script := append([]byte{0x6a, 40}, jobMarker...)
	return append(script, commitment[:]...)
}

// CommitmentScriptForJob returns the version-2 commitment to the exact job ID.
// It validates only the identifier's encoding, not issuance, expiry or birth.
// Job IDs are SHA256 outputs, so an all-zero 32-byte value is allowed.
func CommitmentScriptForJob(context Context, gymID, jobID string) ([]byte, error) {
	parsed, err := parseContext(context)
	if err != nil {
		return nil, err
	}
	gym, err := decode32("gym ID", gymID, true)
	if err != nil {
		return nil, err
	}
	job, err := decode32("job ID", jobID, false)
	if err != nil {
		return nil, err
	}
	return jobCommitmentScript(parsed.policy, gym, job), nil
}

func proofJob(proof Proof) ([32]byte, error) {
	switch proof.Version {
	case Version1:
		if proof.JobID != "" {
			return [32]byte{}, fmt.Errorf("version-1 proof must not carry a job ID")
		}
		return [32]byte{}, nil
	case Version2:
		return decode32("job ID", proof.JobID, false)
	default:
		return [32]byte{}, fmt.Errorf("unsupported proof version %d", proof.Version)
	}
}

// wireHash converts strict display-order bytes to the internal wire order.
func wireHash(display [32]byte) chainhash.Hash {
	var hash chainhash.Hash
	for i := range display {
		hash[i] = display[len(display)-1-i]
	}
	return hash
}

// skipBytes checks the length before seeking. In particular, attacker-provided
// CompactSize values never become allocations or unchecked integer casts.
func skipBytes(reader *bytes.Reader, count uint64) error {
	if count > uint64(reader.Len()) {
		return io.ErrUnexpectedEOF
	}
	_, err := reader.Seek(int64(count), io.SeekCurrent)
	return err
}

// preflightCoinbase checks every variable count before the wire decoder can
// allocate a script buffer from a claimed length. The raw size bound alone
// does not prevent wire's arena from allocating before a short read fails.
func preflightCoinbase(raw []byte) error {
	reader := bytes.NewReader(raw)
	if err := skipBytes(reader, 4); err != nil {
		return err
	}
	inputs, err := wire.ReadVarInt(reader, 0)
	if err != nil {
		return err
	}
	if inputs != 1 {
		return fmt.Errorf("coinbase must have exactly one input and no witness encoding")
	}
	if err := skipBytes(reader, 36); err != nil {
		return err
	}
	scriptSize, err := wire.ReadVarInt(reader, 0)
	if err != nil {
		return err
	}
	if scriptSize < 2 || scriptSize > 100 {
		return fmt.Errorf("coinbase scriptSig length must be in [2,100]")
	}
	if err := skipBytes(reader, scriptSize+4); err != nil {
		return err
	}
	outputs, err := wire.ReadVarInt(reader, 0)
	if err != nil {
		return err
	}
	if outputs == 0 || outputs > maxOutputs {
		return fmt.Errorf("coinbase output count must be in [1,%d]", maxOutputs)
	}
	for i := uint64(0); i < outputs; i++ {
		if err := skipBytes(reader, 8); err != nil {
			return err
		}
		size, err := wire.ReadVarInt(reader, 0)
		if err != nil {
			return err
		}
		if err := skipBytes(reader, size); err != nil {
			return err
		}
	}
	if err := skipBytes(reader, 4); err != nil {
		return err
	}
	if reader.Len() != 0 {
		return fmt.Errorf("coinbase contains trailing bytes")
	}
	return nil
}

// heightPrefix matches canonical script integer encoding, including OP_1
// through OP_16 for the initial heights and a sign-padding byte when needed.
func heightPrefix(height uint32) []byte {
	if height >= 1 && height <= 16 {
		return []byte{byte(0x50 + height)}
	}
	var encoded [5]byte
	count := 0
	for height != 0 {
		encoded[count] = byte(height)
		count++
		height >>= 8
	}
	if encoded[count-1]&0x80 != 0 {
		count++
	}
	return append([]byte{byte(count)}, encoded[:count]...)
}

// isFamilyOutput identifies the reserved family even when a nonminimal push
// or an invalid declared size surrounds it. Such outputs must subsequently
// match the complete canonical script exactly or verification fails.
func isFamilyOutput(script []byte) bool {
	if len(script) < 2 || script[0] != 0x6a {
		return false
	}
	offset := 2
	var length uint64
	switch opcode := script[1]; {
	case opcode <= 75:
		length = uint64(opcode)
	case opcode == 0x4c:
		if len(script) < 3 {
			return false
		}
		length, offset = uint64(script[2]), 3
	case opcode == 0x4d:
		if len(script) < 4 {
			return false
		}
		length, offset = uint64(binary.LittleEndian.Uint16(script[2:4])), 4
	case opcode == 0x4e:
		if len(script) < 6 {
			return false
		}
		length, offset = uint64(binary.LittleEndian.Uint32(script[2:6])), 6
	default:
		return false
	}
	return length >= uint64(len(markerFamily)) &&
		bytes.HasPrefix(script[offset:], []byte(markerFamily))
}

func validateCoinbase(raw, expectedScript []byte, height uint32) (*wire.MsgTx, error) {
	if err := preflightCoinbase(raw); err != nil {
		return nil, fmt.Errorf("invalid stripped coinbase: %w", err)
	}
	var tx wire.MsgTx
	reader := bytes.NewReader(raw)
	if err := tx.DeserializeNoWitness(reader); err != nil {
		return nil, fmt.Errorf("decode coinbase: %w", err)
	}
	var serialized bytes.Buffer
	if err := tx.SerializeNoWitness(&serialized); err != nil {
		return nil, fmt.Errorf("serialize coinbase: %w", err)
	}
	if reader.Len() != 0 || tx.HasWitness() || !bytes.Equal(raw, serialized.Bytes()) {
		return nil, fmt.Errorf("coinbase must use canonical stripped encoding")
	}
	input := tx.TxIn[0]
	if input.PreviousOutPoint.Hash != (chainhash.Hash{}) || input.PreviousOutPoint.Index != ^uint32(0) {
		return nil, fmt.Errorf("coinbase input must use the null previous outpoint")
	}
	if !bytes.HasPrefix(input.SignatureScript, heightPrefix(height)) {
		return nil, fmt.Errorf("coinbase must begin with canonical height %d", height)
	}
	var total int64
	commitments := 0
	for _, output := range tx.TxOut {
		if output.Value < 0 || output.Value > maxMoney || output.Value > maxMoney-total {
			return nil, fmt.Errorf("coinbase output value or total is outside the money range")
		}
		total += output.Value
		if !isFamilyOutput(output.PkScript) {
			continue
		}
		commitments++
		if output.Value != 0 || !bytes.Equal(output.PkScript, expectedScript) {
			return nil, fmt.Errorf("prospect commitment must match the canonical zero-value output")
		}
	}
	if commitments != 1 {
		return nil, fmt.Errorf("coinbase must contain exactly one prospect commitment")
	}
	return &tx, nil
}

// hashMu bounds this package's active Argon arenas, including independent
// callers. It does not bound their queues or hashing performed by the node.
var hashMu sync.Mutex

func hashHeader(header *wire.BlockHeader) chainhash.Hash {
	hashMu.Lock()
	defer hashMu.Unlock()
	return header.BlockHash()
}

func meetsTarget(hash chainhash.Hash, target [32]byte) bool {
	// The target uses unsigned display order; hashes use internal wire order.
	for i := range target {
		value := hash[len(hash)-1-i]
		if value != target[i] {
			return value < target[i]
		}
	}
	return true
}

// ValidateProofStructure checks the existing bounded context, header,
// coinbase commitment and Merkle path without hashing proof work. It proves
// neither proof-of-work nor context provenance and returns no Verified value.
func ValidateProofStructure(context Context, proof Proof) error {
	_, err := prepareProof(context, proof)
	return err
}

type preparedProof struct {
	context parsedContext
	gym     [32]byte
	header  wire.BlockHeader
}

func prepareProof(context Context, proof Proof) (preparedProof, error) {
	parsed, err := parseContext(context)
	if err != nil {
		return preparedProof{}, err
	}
	job, err := proofJob(proof)
	if err != nil {
		return preparedProof{}, err
	}
	gym, err := decode32("gym ID", proof.GymID, true)
	if err != nil {
		return preparedProof{}, err
	}
	if len(proof.Header) != 160 {
		return preparedProof{}, fmt.Errorf("header must contain exactly 160 hexadecimal characters")
	}
	headerBytes, err := hex.DecodeString(proof.Header)
	if err != nil {
		return preparedProof{}, fmt.Errorf("invalid header: %w", err)
	}
	if len(proof.Coinbase) == 0 || len(proof.Coinbase) > maxCoinbaseSize*2 || len(proof.Coinbase)%2 != 0 {
		return preparedProof{}, fmt.Errorf("coinbase must encode 1 to %d bytes", maxCoinbaseSize)
	}
	coinbase, err := hex.DecodeString(proof.Coinbase)
	if err != nil {
		return preparedProof{}, fmt.Errorf("invalid coinbase: %w", err)
	}
	if len(proof.MerkleBranch) > maxMerkleBranch {
		return preparedProof{}, fmt.Errorf("Merkle branch exceeds %d siblings", maxMerkleBranch)
	}
	var siblings [maxMerkleBranch]chainhash.Hash
	for i, text := range proof.MerkleBranch {
		display, err := decode32("Merkle sibling", text, false)
		if err != nil {
			return preparedProof{}, err
		}
		siblings[i] = wireHash(display)
	}
	var header wire.BlockHeader
	if err := header.Deserialize(bytes.NewReader(headerBytes)); err != nil {
		return preparedProof{}, fmt.Errorf("decode header: %w", err)
	}
	if header.PrevBlock != wireHash(parsed.parent) {
		return preparedProof{}, fmt.Errorf("header does not match supplied parent")
	}
	if header.Bits != context.ExpectedBits {
		return preparedProof{}, fmt.Errorf("header does not match supplied expected bits")
	}
	timestamp := uint32(header.Timestamp.Unix())
	if timestamp < context.MinTimestamp || timestamp > context.MaxTimestamp {
		return preparedProof{}, fmt.Errorf("header timestamp is outside supplied interval")
	}
	expectedScript := commitmentScript(parsed.policy, gym)
	if proof.Version == Version2 {
		expectedScript = jobCommitmentScript(parsed.policy, gym, job)
	}
	tx, err := validateCoinbase(coinbase, expectedScript, context.ParentHeight+1)
	if err != nil {
		return preparedProof{}, err
	}
	current := tx.TxHash()
	for _, sibling := range siblings[:len(proof.MerkleBranch)] {
		if current == sibling {
			return preparedProof{}, fmt.Errorf("index-zero Merkle path has an equal sibling")
		}
		var pair [64]byte
		copy(pair[:32], current[:])
		copy(pair[32:], sibling[:])
		current = chainhash.DoubleHashH(pair[:])
	}
	if current != header.MerkleRoot {
		return preparedProof{}, fmt.Errorf("coinbase Merkle root does not match header")
	}
	return preparedProof{context: parsed, gym: gym, header: header}, nil
}

// Verify checks a gym-bound work proof against supplied trusted assertions.
// Success does not authenticate that context, gym control, birth time, or the
// other transactions. All structural checks precede a serialized production
// Argon call; callers remain responsible for admission and queue limits.
// Version-2 success checks the job commitment, not issuance or freshness.
func Verify(context Context, proof Proof) (Verified, error) {
	var empty Verified
	if wire.TestPoW {
		return empty, fmt.Errorf("prospect verification requires production PoW; slugtestpow is unsupported")
	}
	prepared, err := prepareProof(context, proof)
	if err != nil {
		return empty, err
	}
	parsed, gym, header := prepared.context, prepared.gym, prepared.header
	workHash := hashHeader(&header)
	if !meetsTarget(workHash, parsed.target) {
		return empty, fmt.Errorf("work hash exceeds supplied prospect target")
	}
	seed, err := fighter.ParseSeed(workHash.String())
	if err != nil {
		return empty, err
	}
	dna, err := fighter.Generate(context.DNAVersion, seed)
	if err != nil {
		return empty, err
	}
	idBytes := append([]byte(idDomain), le32(context.Network)...)
	idBytes = append(idBytes, parsed.genesis[:]...)
	idBytes = append(idBytes, seed[:]...)
	id := sha256.Sum256(idBytes)
	return Verified{
		Version: proof.Version, Verification: "supplied-context", Network: context.Network,
		Genesis: hex.EncodeToString(parsed.genesis[:]), ProspectID: hex.EncodeToString(id[:]),
		PolicyID: hex.EncodeToString(parsed.policy[:]), GymID: hex.EncodeToString(gym[:]),
		WorkHash: workHash.String(), ParentHash: hex.EncodeToString(parsed.parent[:]),
		ParentHeight: context.ParentHeight, DNA: dna,
	}, nil
}
