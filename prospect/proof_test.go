//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospect

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

type proofFixture struct {
	Name             string   `json:"name"`
	Context          Context  `json:"context"`
	Proof            Proof    `json:"proof"`
	Expected         Verified `json:"expected"`
	CommitmentScript string   `json:"commitment_script"`
}

func proofFixtures(t *testing.T) []proofFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/proofs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []proofFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) < 2 {
		t.Fatal("independent fixtures must cover empty and nonempty Merkle paths")
	}
	return fixtures
}

func TestIndependentProofs(t *testing.T) {
	for _, fixture := range proofFixtures(t) {
		t.Run(fixture.Name, func(t *testing.T) {
			script, err := CommitmentScript(fixture.Context, fixture.Proof.GymID)
			if err != nil || hex.EncodeToString(script) != fixture.CommitmentScript {
				t.Fatalf("commitment = %x, %v; want %s", script, err, fixture.CommitmentScript)
			}
			got, err := Verify(fixture.Context, fixture.Proof)
			if err != nil {
				t.Fatal(err)
			}
			if got != fixture.Expected {
				t.Fatalf("verified record differs from independent oracle:\ngot  %+v\nwant %+v", got, fixture.Expected)
			}
			// These fixtures deliberately miss the network target of one.
			// Prospect qualification must use the separately bound target.
			if got.WorkHash == strings.Repeat("0", 63)+"1" {
				t.Fatal("fixture must miss its network target")
			}

			upperContext, upperProof := fixture.Context, fixture.Proof
			upperContext.Genesis = strings.ToUpper(upperContext.Genesis)
			upperContext.ProspectTarget = strings.ToUpper(upperContext.ProspectTarget)
			upperContext.ParentHash = strings.ToUpper(upperContext.ParentHash)
			upperProof.GymID = strings.ToUpper(upperProof.GymID)
			upperProof.Header = strings.ToUpper(upperProof.Header)
			upperProof.Coinbase = strings.ToUpper(upperProof.Coinbase)
			upperProof.MerkleBranch = append([]string{}, upperProof.MerkleBranch...)
			for i := range upperProof.MerkleBranch {
				upperProof.MerkleBranch[i] = strings.ToUpper(upperProof.MerkleBranch[i])
			}
			upper, err := Verify(upperContext, upperProof)
			if err != nil || upper != got {
				t.Fatalf("case normalization changed identity or result: %+v, %v", upper, err)
			}
		})
	}
}

func TestContextValidation(t *testing.T) {
	fixture := proofFixtures(t)[0]
	mutations := []struct {
		name string
		edit func(*Context)
	}{
		{"version", func(c *Context) { c.Version = 2 }},
		{"network", func(c *Context) { c.Network = 0 }},
		{"DNA version", func(c *Context) { c.DNAVersion = 2 }},
		{"height overflow", func(c *Context) { c.ParentHeight = 2147483647 }},
		{"bits", func(c *Context) { c.ExpectedBits = 0 }},
		{"inverted time", func(c *Context) { c.MinTimestamp = 2; c.MaxTimestamp = 1 }},
		{"zero genesis", func(c *Context) { c.Genesis = strings.Repeat("0", 64) }},
		{"short genesis", func(c *Context) { c.Genesis = "01" }},
		{"nonhex genesis", func(c *Context) { c.Genesis = strings.Repeat("z", 64) }},
		{"zero target", func(c *Context) { c.ProspectTarget = strings.Repeat("0", 64) }},
		{"short target", func(c *Context) { c.ProspectTarget = "1" }},
		{"zero parent", func(c *Context) { c.ParentHash = strings.Repeat("0", 64) }},
		{"prefixed parent", func(c *Context) { c.ParentHash = "0x" + c.ParentHash }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			context := fixture.Context
			mutation.edit(&context)
			if _, err := CommitmentScript(context, fixture.Proof.GymID); err == nil {
				t.Fatal("commitment helper accepted invalid context")
			}
			got, err := Verify(context, fixture.Proof)
			if err == nil || got != (Verified{}) {
				t.Fatalf("invalid context returned record %+v, error %v", got, err)
			}
		})
	}
	for _, gym := range []string{"", "01", strings.Repeat("0", 64), strings.Repeat("g", 64), " " + fixture.Proof.GymID} {
		if _, err := CommitmentScript(fixture.Context, gym); err == nil {
			t.Errorf("accepted gym ID %q", gym)
		}
	}
}

func TestPolicyBinding(t *testing.T) {
	fixture := proofFixtures(t)[0]
	for _, mutation := range []struct {
		name string
		edit func(*Context, *Proof)
	}{
		{"network", func(c *Context, _ *Proof) { c.Network++ }},
		{"genesis", func(c *Context, _ *Proof) { c.Genesis = strings.Repeat("a", 64) }},
		{"target", func(c *Context, _ *Proof) { c.ProspectTarget = strings.Repeat("e", 64) }},
		{"gym", func(_ *Context, p *Proof) { p.GymID = strings.Repeat("b", 64) }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			context, proof := fixture.Context, fixture.Proof
			mutation.edit(&context, &proof)
			got, err := Verify(context, proof)
			if err == nil || !strings.Contains(err.Error(), "commitment") || got != (Verified{}) {
				t.Fatalf("changed binding returned %+v, %v", got, err)
			}
		})
	}
}

func changeCoinbase(t *testing.T, proof Proof, mutate func(*wire.MsgTx)) Proof {
	t.Helper()
	raw, err := hex.DecodeString(proof.Coinbase)
	if err != nil {
		t.Fatal(err)
	}
	var tx wire.MsgTx
	if err := tx.DeserializeNoWitness(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	mutate(&tx)
	var buffer bytes.Buffer
	if err := tx.SerializeNoWitness(&buffer); err != nil {
		t.Fatal(err)
	}
	proof.Coinbase = hex.EncodeToString(buffer.Bytes())
	return proof
}

func TestCoinbaseCommitmentRejection(t *testing.T) {
	fixture := proofFixtures(t)[0]
	canonical, err := hex.DecodeString(fixture.CommitmentScript)
	if err != nil {
		t.Fatal(err)
	}
	outputWith := func(script []byte) func(*wire.MsgTx) {
		return func(tx *wire.MsgTx) { tx.TxOut = []*wire.TxOut{{PkScript: script}} }
	}
	for _, mutation := range []struct {
		name string
		edit func(*wire.MsgTx)
	}{
		{"missing", outputWith([]byte{0x51})},
		{"wrong family version", outputWith(append([]byte{0x6a, 40}, append([]byte("SLUGPR02"), canonical[10:]...)...))},
		{"trailing script", outputWith(append(append([]byte{}, canonical...), 0x00))},
		{"PUSHDATA1", outputWith(append([]byte{0x6a, 0x4c, 40}, canonical[2:]...))},
		{"PUSHDATA2", outputWith(append([]byte{0x6a, 0x4d, 40, 0}, canonical[2:]...))},
		{"PUSHDATA4", outputWith(append([]byte{0x6a, 0x4e, 40, 0, 0, 0}, canonical[2:]...))},
		{"huge push claim", outputWith(append([]byte{0x6a, 0x4e, 0xff, 0xff, 0xff, 0xff}, canonical[2:]...))},
		{"short family", outputWith(append([]byte{0x6a, 6}, []byte("SLUGPR")...))},
		{"duplicate", func(tx *wire.MsgTx) { tx.TxOut = []*wire.TxOut{{PkScript: canonical}, {PkScript: canonical}} }},
		{"second gym", func(tx *wire.MsgTx) {
			other, err := CommitmentScript(fixture.Context, strings.Repeat("c", 64))
			if err != nil {
				t.Fatal(err)
			}
			tx.TxOut = []*wire.TxOut{{PkScript: canonical}, {PkScript: other}}
		}},
		{"nonzero commitment value", func(tx *wire.MsgTx) { tx.TxOut = []*wire.TxOut{{Value: 1, PkScript: canonical}} }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			proof := changeCoinbase(t, fixture.Proof, mutation.edit)
			got, err := Verify(fixture.Context, proof)
			if err == nil || !strings.Contains(err.Error(), "commitment") || got != (Verified{}) {
				t.Fatalf("bad commitment returned %+v, %v", got, err)
			}
		})
	}

	// A different OP_RETURN family remains permitted by this proof format.
	proof := changeCoinbase(t, fixture.Proof, func(tx *wire.MsgTx) {
		tx.TxOut = append(tx.TxOut, &wire.TxOut{PkScript: append([]byte{0x6a, 36, 0xaa, 0x21, 0xa9, 0xed}, make([]byte, 32)...)})
	})
	raw, _ := hex.DecodeString(proof.Coinbase)
	if _, err := validateCoinbase(raw, canonical, fixture.Context.ParentHeight+1); err != nil {
		t.Fatalf("unrelated witness commitment rejected: %v", err)
	}
}

func TestCoinbaseStructure(t *testing.T) {
	fixture := proofFixtures(t)[0]
	for _, mutation := range []struct {
		name string
		edit func(*wire.MsgTx)
	}{
		{"nonzero previous hash", func(tx *wire.MsgTx) { tx.TxIn[0].PreviousOutPoint.Hash[0] = 1 }},
		{"non-null previous index", func(tx *wire.MsgTx) { tx.TxIn[0].PreviousOutPoint.Index = 0 }},
		{"no inputs", func(tx *wire.MsgTx) { tx.TxIn = nil }},
		{"two inputs", func(tx *wire.MsgTx) { tx.TxIn = append(tx.TxIn, tx.TxIn[0]) }},
		{"no outputs", func(tx *wire.MsgTx) { tx.TxOut = nil }},
		{"too many outputs", func(tx *wire.MsgTx) {
			tx.TxOut = make([]*wire.TxOut, 257)
			for i := range tx.TxOut {
				tx.TxOut[i] = &wire.TxOut{}
			}
		}},
		{"short scriptSig", func(tx *wire.MsgTx) { tx.TxIn[0].SignatureScript = []byte{0x51} }},
		{"long scriptSig", func(tx *wire.MsgTx) { tx.TxIn[0].SignatureScript = make([]byte, 101) }},
		{"wrong height", func(tx *wire.MsgTx) { tx.TxIn[0].SignatureScript = []byte{1, 63, 0} }},
		{"nonminimal height", func(tx *wire.MsgTx) { tx.TxIn[0].SignatureScript = []byte{2, 62, 0} }},
		{"negative value", func(tx *wire.MsgTx) { tx.TxOut[0].Value = -1 }},
		{"excess value", func(tx *wire.MsgTx) { tx.TxOut[0].Value = maxMoney + 1 }},
		{"excess total", func(tx *wire.MsgTx) { tx.TxOut = append(tx.TxOut, &wire.TxOut{Value: maxMoney}, &wire.TxOut{Value: 1}) }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			proof := changeCoinbase(t, fixture.Proof, mutation.edit)
			got, err := Verify(fixture.Context, proof)
			if err == nil || got != (Verified{}) {
				t.Fatalf("invalid coinbase returned %+v, %v", got, err)
			}
			if strings.Contains(err.Error(), "Merkle") || strings.Contains(err.Error(), "work hash") {
				t.Fatalf("coinbase failed too late: %v", err)
			}
		})
	}
}

func TestPreflightLengths(t *testing.T) {
	fixture := proofFixtures(t)[0]
	raw, _ := hex.DecodeString(fixture.Proof.Coinbase)
	for length := 0; length < len(raw); length++ {
		if err := preflightCoinbase(raw[:length]); err == nil {
			t.Fatalf("accepted coinbase truncated at %d", length)
		}
	}
	if err := preflightCoinbase(append(append([]byte{}, raw...), 0)); err == nil {
		t.Fatal("accepted trailing data")
	}
	if err := preflightCoinbase(raw); err != nil {
		t.Fatal(err)
	}
	// Replace the one-input CompactSize with a nonminimal representation.
	nonminimal := append(append([]byte{}, raw[:4]...), 0xfd, 1, 0)
	nonminimal = append(nonminimal, raw[5:]...)
	if err := preflightCoinbase(nonminimal); err == nil {
		t.Fatal("accepted noncanonical CompactSize")
	}
	// Witness marker/flag cannot be interpreted as a stripped coinbase.
	witness := append(append([]byte{}, raw[:4]...), 0, 1)
	witness = append(witness, raw[4:]...)
	if err := preflightCoinbase(witness); err == nil {
		t.Fatal("accepted witness encoding")
	}
	// Construct tiny payloads claiming 4 MiB or uint64-max script lengths.
	// Neither may reach the allocating wire decoder.
	for _, claim := range []uint64{4000000, ^uint64(0)} {
		var payload bytes.Buffer
		payload.Write(raw[:41]) // Version, input count, null previous outpoint.
		_ = wire.WriteVarInt(&payload, 0, claim)
		if err := preflightCoinbase(payload.Bytes()); err == nil {
			t.Fatalf("accepted scriptSig length %d", claim)
		}
		payload.Reset()
		payload.Write(raw[:41])
		payload.Write([]byte{2, 0x51, 0})
		payload.Write(make([]byte, 4))
		payload.WriteByte(1)
		payload.Write(make([]byte, 8))
		_ = wire.WriteVarInt(&payload, 0, claim)
		if err := preflightCoinbase(payload.Bytes()); err == nil {
			t.Fatalf("accepted output script length %d", claim)
		}
	}
}

func TestCanonicalHeight(t *testing.T) {
	for _, test := range []struct {
		height uint32
		hex    string
	}{
		{1, "51"}, {16, "60"}, {17, "0111"}, {127, "017f"},
		{128, "028000"}, {255, "02ff00"}, {256, "020001"},
		{32768, "03008000"}, {2147483647, "04ffffff7f"},
	} {
		if got := hex.EncodeToString(heightPrefix(test.height)); got != test.hex {
			t.Errorf("height %d = %s; want %s", test.height, got, test.hex)
		}
	}
}

func TestTargetBoundary(t *testing.T) {
	var target [32]byte
	for i := range target {
		target[i] = byte(i + 1)
	}
	// Literal reversal ensures this test does not obtain its input through
	// the production conversion helper being exercised by verification.
	hash := chainhash.Hash{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	if !meetsTarget(hash, target) {
		t.Fatal("exact target rejected")
	}
	hash[0]--
	if !meetsTarget(hash, target) {
		t.Fatal("target minus one rejected")
	}
	hash[0] += 2
	if meetsTarget(hash, target) {
		t.Fatal("target plus one accepted")
	}
	hash[0], hash[31] = 0, 2
	if meetsTarget(hash, target) {
		t.Fatal("comparison prioritized low-order bytes")
	}
}

func TestHeaderAndMerkleRejectionBeforeHash(t *testing.T) {
	fixtures := proofFixtures(t)
	fixture := fixtures[0]
	header, _ := hex.DecodeString(fixture.Proof.Header)
	timestamp := binary.LittleEndian.Uint32(header[68:72])
	mutations := []struct {
		name string
		edit func(*Context, *Proof)
	}{
		{"proof version", func(_ *Context, p *Proof) { p.Version = 2 }},
		{"short header", func(_ *Context, p *Proof) { p.Header = p.Header[:158] }},
		{"oversized header", func(_ *Context, p *Proof) { p.Header += "00" }},
		{"nonhex header", func(_ *Context, p *Proof) { p.Header = strings.Repeat("z", 160) }},
		{"oversized coinbase", func(_ *Context, p *Proof) { p.Coinbase = strings.Repeat("00", maxCoinbaseSize+1) }},
		{"odd coinbase", func(_ *Context, p *Proof) { p.Coinbase += "0" }},
		{"nonhex coinbase", func(_ *Context, p *Proof) { p.Coinbase = "zz" }},
		{"wrong parent", func(c *Context, _ *Proof) { c.ParentHash = strings.Repeat("c", 64) }},
		{"wrong bits", func(c *Context, _ *Proof) { c.ExpectedBits++ }},
		{"too early", func(c *Context, _ *Proof) { c.MinTimestamp = timestamp + 1 }},
		{"too late", func(c *Context, _ *Proof) { c.MaxTimestamp = timestamp - 1 }},
		{"wrong root", func(_ *Context, p *Proof) {
			data := append([]byte{}, header...)
			data[36] ^= 1
			p.Header = hex.EncodeToString(data)
		}},
		{"long branch", func(_ *Context, p *Proof) { p.MerkleBranch = make([]string, 21) }},
		{"short sibling", func(_ *Context, p *Proof) { p.MerkleBranch = []string{"01"} }},
		{"equal sibling", func(_ *Context, p *Proof) {
			var tx wire.MsgTx
			raw, _ := hex.DecodeString(p.Coinbase)
			_ = tx.DeserializeNoWitness(bytes.NewReader(raw))
			p.MerkleBranch = []string{tx.TxHash().String()}
		}},
		{"reversed branch", func(_ *Context, p *Proof) {
			*p = fixtures[1].Proof
			p.MerkleBranch = append([]string{}, p.MerkleBranch...)
			p.MerkleBranch[0], p.MerkleBranch[1] = p.MerkleBranch[1], p.MerkleBranch[0]
		}},
	}
	// Holding the only hash gate proves these failures occur before Argon.
	hashMu.Lock()
	defer hashMu.Unlock()
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			context, proof := fixture.Context, fixture.Proof
			mutation.edit(&context, &proof)
			type result struct {
				record Verified
				err    error
			}
			done := make(chan result, 1)
			go func() { got, err := Verify(context, proof); done <- result{got, err} }()
			select {
			case got := <-done:
				if got.err == nil || got.record != (Verified{}) {
					t.Fatalf("invalid proof returned %+v, %v", got.record, got.err)
				}
			case <-time.After(time.Second):
				t.Fatal("structural rejection waited for expensive hashing")
			}
		})
	}
}

func TestTimestampEndpoints(t *testing.T) {
	fixture := proofFixtures(t)[0]
	header, _ := hex.DecodeString(fixture.Proof.Header)
	timestamp := binary.LittleEndian.Uint32(header[68:72])
	fixture.Context.MinTimestamp, fixture.Context.MaxTimestamp = timestamp, timestamp
	got, err := Verify(fixture.Context, fixture.Proof)
	if err != nil || got != fixture.Expected {
		t.Fatalf("inclusive timestamp boundary changed result: %+v, %v", got, err)
	}
}

func TestBoundWorkBelowTarget(t *testing.T) {
	fixture := proofFixtures(t)[0]
	fixture.Context.ProspectTarget = strings.Repeat("0", 63) + "1"
	script, err := CommitmentScript(fixture.Context, fixture.Proof.GymID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Proof = changeCoinbase(t, fixture.Proof, func(tx *wire.MsgTx) {
		tx.TxOut = []*wire.TxOut{{PkScript: script}}
	})
	coinbase, _ := hex.DecodeString(fixture.Proof.Coinbase)
	txid := chainhash.DoubleHashH(coinbase)
	header, _ := hex.DecodeString(fixture.Proof.Header)
	copy(header[36:68], txid[:])
	fixture.Proof.Header = hex.EncodeToString(header)
	got, err := Verify(fixture.Context, fixture.Proof)
	if err == nil || !strings.Contains(err.Error(), "work hash exceeds") || got != (Verified{}) {
		t.Fatalf("below-target work returned %+v, %v", got, err)
	}
}

func TestConcurrentVerificationGate(t *testing.T) {
	fixture := proofFixtures(t)[0]
	hashMu.Lock()
	locked := true
	defer func() {
		if locked {
			hashMu.Unlock()
		}
	}()
	type result struct {
		record Verified
		err    error
	}
	done := make(chan result, 4)
	started := make(chan struct{}, 4)
	for i := 0; i < cap(done); i++ {
		go func() {
			started <- struct{}{}
			got, err := Verify(fixture.Context, fixture.Proof)
			done <- result{got, err}
		}()
	}
	for i := 0; i < cap(done); i++ {
		<-started
	}
	select {
	case got := <-done:
		t.Fatalf("verification bypassed the shared hash gate: %+v", got)
	case <-time.After(20 * time.Millisecond):
	}
	hashMu.Unlock()
	locked = false
	for i := 0; i < cap(done); i++ {
		select {
		case got := <-done:
			if got.err != nil || !reflect.DeepEqual(got.record, fixture.Expected) {
				t.Fatalf("concurrent verification = %+v, %v", got.record, got.err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("verification did not finish after hash gate released")
		}
	}
}
