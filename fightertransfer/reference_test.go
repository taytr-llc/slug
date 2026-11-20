// Copyright (c) 2026 The SLUG developers. ISC license.
package fightertransfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

const referenceSHA = "365e2997171b5f06706d628866f2cbc64cf0d7a3ac4205015def259a5340f4af"

type referenceValue struct {
	Name       string   `json:"name"`
	Pin        string   `json:"expected_pin"`
	Prior      string   `json:"expected_prior"`
	Sender     string   `json:"sender_gym"`
	Commit     string   `json:"commit_raw"`
	Spend      string   `json:"spend_raw"`
	Parents    []string `json:"funding_transactions"`
	Intent     string   `json:"intent"`
	Digest     string   `json:"authorization_digest"`
	Envelope   string   `json:"envelope"`
	Redeem     string   `json:"redeem"`
	ID         string   `json:"transfer_id"`
	CommitID   string   `json:"commit_txid"`
	SpendID    string   `json:"spend_txid"`
	IsTransfer bool     `json:"is_transfer"`
}
type referenceNegative struct {
	Name  string         `json:"name"`
	Value referenceValue `json:"value"`
}
type referenceTuple struct {
	Name     string `json:"name"`
	Prevouts []struct {
		Hash  string `json:"hash_wire"`
		Index uint32 `json:"index"`
	} `json:"prevouts"`
}
type referenceCorpus struct {
	Positives   []referenceValue    `json:"positives"`
	Negatives   []referenceNegative `json:"negatives"`
	Unavailable []referenceNegative `json:"unavailable"`
	Tuples      []referenceTuple    `json:"tuple_refusals"`
}

func readReference(t *testing.T) referenceCorpus {
	t.Helper()
	raw, e := os.ReadFile("testdata/carrier-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != referenceSHA {
		t.Fatal("independent reference changed")
	}
	var c referenceCorpus
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	if len(c.Positives) != 12 || len(c.Negatives) != 31 || len(c.Unavailable) != 2 || len(c.Tuples) != 4 {
		t.Fatal("reference cases omitted")
	}
	return c
}
func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func supplied(t *testing.T, v referenceValue) (Expected, Evidence) {
	t.Helper()
	var expected Expected
	copy(expected.PinID[:], unhex(t, v.Pin))
	copy(expected.PriorStateID[:], unhex(t, v.Prior))
	copy(expected.SenderGym[:], unhex(t, v.Sender))
	e := Evidence{CommitRaw: unhex(t, v.Commit), SpendRaw: unhex(t, v.Spend)}
	for _, p := range v.Parents {
		e.FundingTransactions = append(e.FundingTransactions, unhex(t, p))
	}
	return expected, e
}
func TestCarrierIndependentReference(t *testing.T) {
	c := readReference(t)
	for _, v := range c.Positives {
		t.Run(v.Name, func(t *testing.T) {
			expected, e := supplied(t, v)
			r, err := ValidateSupplied(context.Background(), expected, e)
			if err != nil {
				t.Fatal(err)
			}
			kind := Refund
			if v.IsTransfer {
				kind = Reveal
			}
			if r.Kind != kind || hex.EncodeToString(r.TransferID[:]) != v.ID || r.CommitTxID.String() != v.CommitID || r.SpendTxID.String() != v.SpendID {
				t.Fatalf("wrong public facts: %+v", r)
			}
			raw, err := MarshalIntent(r.Envelope.Intent)
			if err != nil || !bytes.Equal(raw, unhex(t, v.Intent)) {
				t.Fatal("independent intent mismatch", err)
			}
			digest, err := AuthorizationDigest(r.Envelope.Intent)
			if err != nil || hex.EncodeToString(digest[:]) != v.Digest {
				t.Fatal("independent digest mismatch", err)
			}
			raw, err = MarshalEnvelope(r.Envelope)
			if err != nil || !bytes.Equal(raw, unhex(t, v.Envelope)) {
				t.Fatal("independent envelope mismatch", err)
			}
			raw, err = RedeemScript(r.Envelope, r.FundingKey)
			if err != nil || !bytes.Equal(raw, unhex(t, v.Redeem)) {
				t.Fatal("independent redeem mismatch", err)
			}
			// Returned facts own their bounded values and cannot alias the supplied bytes.
			before := r
			for i := range e.CommitRaw {
				e.CommitRaw[i] = 0
			}
			for i := range e.SpendRaw {
				e.SpendRaw[i] = 0
			}
			if r.Envelope != before.Envelope || r.ReturnScript != before.ReturnScript || r.FundingKey != before.FundingKey {
				t.Fatal("supplied buffer retained")
			}
		})
	}
	for _, v := range c.Negatives {
		t.Run(v.Name, func(t *testing.T) {
			x, e := supplied(t, v.Value)
			if _, err := ValidateSupplied(context.Background(), x, e); !errors.Is(err, ErrInvalid) {
				t.Fatalf("wanted invalid, got %v", err)
			}
		})
	}
	for _, v := range c.Unavailable {
		t.Run(v.Name, func(t *testing.T) {
			x, e := supplied(t, v.Value)
			if _, err := ValidateSupplied(context.Background(), x, e); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("wanted unavailable, got %v", err)
			}
		})
	}
	for _, v := range c.Tuples {
		t.Run(v.Name, func(t *testing.T) {
			var ops []wire.OutPoint
			for _, p := range v.Prevouts {
				var h chainhash.Hash
				copy(h[:], unhex(t, p.Hash))
				ops = append(ops, wire.OutPoint{Hash: h, Index: p.Index})
			}
			if _, err := FundingDigest(ops); !errors.Is(err, ErrInvalid) {
				t.Fatalf("wanted invalid tuple, got %v", err)
			}
		})
	}
}

func TestCarrierResourceFailuresAreNotInvalidity(t *testing.T) {
	v := readReference(t).Positives[0]
	expected, valid := supplied(t, v)
	for name, e := range map[string]Evidence{
		"one_raw":      {CommitRaw: make([]byte, MaxRawSize+1), SpendRaw: valid.SpendRaw},
		"aggregate":    {CommitRaw: valid.CommitRaw, SpendRaw: valid.SpendRaw, FundingTransactions: [][]byte{make([]byte, MaxRawSize), make([]byte, MaxRawSize), make([]byte, MaxRawSize), make([]byte, MaxRawSize), make([]byte, MaxRawSize)}},
		"parent_count": {CommitRaw: valid.CommitRaw, SpendRaw: valid.SpendRaw, FundingTransactions: make([][]byte, MaxInputs+1)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateSupplied(context.Background(), expected, e); !errors.Is(err, ErrCapacity) || errors.Is(err, ErrInvalid) {
				t.Fatalf("resource failure misclassified: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ValidateSupplied(ctx, expected, valid); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored", err)
	}
	if _, err := ValidateSupplied(nil, expected, valid); !errors.Is(err, ErrInvalid) {
		t.Fatal("nil context accepted", err)
	}
	if _, err := ValidateSupplied(context.Background(), expected, Evidence{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing evidence called invalid", err)
	}
}
func TestCarrierPrewalkRejectsForgedLengths(t *testing.T) {
	// A claimed input/witness count cannot allocate before its containing bytes
	// prove the minimum framing. These aren't alternate valid carrier vectors.
	raws := [][]byte{{1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, {1, 0, 0, 0, 0, 1, 1}}
	for _, raw := range raws {
		if _, err := scanTransaction(context.Background(), raw, parentMode, nil); !errors.Is(err, ErrInvalid) {
			t.Fatal("forged count accepted", err)
		}
	}
	script := []byte{0x4e, 0xff, 0xff, 0xff, 0xff}
	if _, _, err := outerElements(context.Background(), script); !errors.Is(err, ErrInvalid) {
		t.Fatal("forged push accepted", err)
	}
}

func TestCarrierDiscoveryUsesPublicDiscriminators(t *testing.T) {
	for _, v := range readReference(t).Positives {
		t.Run(v.Name, func(t *testing.T) {
			_, e := supplied(t, v)
			commit, err := Discover(context.Background(), e.CommitRaw)
			if err != nil || !commit.CommitLocator || commit.RevealLocator || commit.EmbeddedEnvelope || commit.TxID.String() != v.CommitID {
				t.Fatalf("commit not discovered from public locator: %+v %v", commit, err)
			}
			spend, err := Discover(context.Background(), e.SpendRaw)
			if err != nil || spend.CommitLocator || !spend.EmbeddedEnvelope || spend.RevealLocator != v.IsTransfer || spend.TxID.String() != v.SpendID || spend.InputCount != 1 || spend.FirstInput.Hash != commit.TxID || spend.FirstInput.Index != 0 {
				t.Fatalf("spend discovery changed refund/transfer distinction: %+v %v", spend, err)
			}
			for _, parent := range e.FundingTransactions {
				if _, err = Discover(context.Background(), parent); !errors.Is(err, ErrNotCarrier) {
					t.Fatalf("ordinary parent invented a carrier: %v", err)
				}
			}
		})
	}
	// Discovery is deliberately not validation: a recognized future/bad version
	// must reach the public recognizer's refusal path, rather than disappear.
	v := readReference(t).Positives[0]
	x, e := supplied(t, v)
	needle := []byte("SLUGFCM1\x01")
	position := bytes.Index(e.CommitRaw, needle)
	if position < 0 {
		t.Fatal("independent commit locator missing")
	}
	e.CommitRaw[position+8] = 2
	if d, err := Discover(context.Background(), e.CommitRaw); err != nil || !d.CommitLocator {
		t.Fatal("malformed recognized locator hidden", err)
	}
	if _, err := ValidateSupplied(context.Background(), x, e); !errors.Is(err, ErrInvalid) {
		t.Fatal("discovery granted validity", err)
	}
}

func TestCarrierOuterStackIncludesP2SHExecution(t *testing.T) {
	v := readReference(t).Positives[0]
	x, e := supplied(t, v)
	var spend wire.MsgTx
	if err := spend.Deserialize(bytes.NewReader(e.SpendRaw)); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), spend.TxIn[0].SignatureScript...)
	for _, prefixCount := range []int{997, 998} {
		spend.TxIn[0].SignatureScript = append(bytes.Repeat([]byte{txscript.OP_0}, prefixCount), original...)
		var encoded bytes.Buffer
		if err := spend.Serialize(&encoded); err != nil {
			t.Fatal(err)
		}
		e.SpendRaw = encoded.Bytes()
		_, err := ValidateSupplied(context.Background(), x, e)
		if prefixCount == 997 && err != nil {
			t.Fatal("999-item outer stack should fit P2SH execution", err)
		}
		if prefixCount == 998 && !errors.Is(err, ErrInvalid) {
			t.Fatal("1000-item outer stack leaves no slot for P2SH hash push", err)
		}
	}
}
