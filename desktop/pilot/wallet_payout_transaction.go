package pilot

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/taytr-llc/slug/poolsettle"
)

// This cursor never allocates from transaction-provided counts. In particular,
// witness stacks are walked in the bounded input rather than materialized.
type payoutTransactionCursor struct {
	raw []byte
	pos int
	bad bool
}

func (c *payoutTransactionCursor) take(n uint64) []byte {
	if c.bad || n > uint64(len(c.raw)-c.pos) {
		c.bad = true
		return nil
	}
	start := c.pos
	c.pos += int(n)
	return c.raw[start:c.pos]
}

func (c *payoutTransactionCursor) count() uint64 {
	b := c.take(1)
	if c.bad {
		return 0
	}
	var n uint64
	switch b[0] {
	case 253:
		v := c.take(2)
		if c.bad {
			return 0
		}
		n = uint64(binary.LittleEndian.Uint16(v))
		c.bad = n < 253
	case 254:
		v := c.take(4)
		if c.bad {
			return 0
		}
		n = uint64(binary.LittleEndian.Uint32(v))
		c.bad = n <= 65535
	case 255:
		v := c.take(8)
		if c.bad {
			return 0
		}
		n = binary.LittleEndian.Uint64(v)
		c.bad = n <= 4294967295
	default:
		n = uint64(b[0])
	}
	return n
}

type walletParsedPayout struct {
	txid            string
	inputs, outputs uint64
	witness         bool
	atoms           int64
	script          []byte // borrowed from the bounded input, never retained by the owner
}

func validateWalletPayoutTransaction(raw []byte, selected walletPayoutSelection) bool {
	if !selected.valid() {
		return false
	}
	result, ok := parseWalletPayoutTransaction(raw, selected.OutputIndex)
	return ok && result.txid == selected.TxID && result.atoms == selected.Atoms && hex.EncodeToString(result.script) == selected.RecipientScript
}

func parseWalletPayoutTransaction(raw []byte, outputIndex uint32) (walletParsedPayout, bool) {
	var result walletParsedPayout
	if len(raw) < 10 || len(raw) > poolsettle.MaxTransactionBytes {
		return walletParsedPayout{}, false
	}
	c := payoutTransactionCursor{raw: raw, pos: 4}
	witness := raw[c.pos] == 0
	if witness {
		marker := c.take(2)
		if c.bad || marker[1] != 1 {
			return walletParsedPayout{}, false
		}
	}
	strippedStart := c.pos
	inputs := c.count()
	if c.bad || inputs < 1 || inputs > 256 || inputs > uint64((len(raw)-c.pos)/41) {
		return walletParsedPayout{}, false
	}
	for i := uint64(0); i < inputs; i++ {
		c.take(36)
		c.take(c.count())
		c.take(4)
		if c.bad {
			return walletParsedPayout{}, false
		}
	}
	outputs := c.count()
	if c.bad || outputs < 1 || outputs > 257 || uint64(outputIndex) >= outputs || outputs > uint64((len(raw)-c.pos)/9) {
		return walletParsedPayout{}, false
	}
	total := uint64(0)
	for i := uint64(0); i < outputs; i++ {
		value := c.take(8)
		if c.bad {
			return walletParsedPayout{}, false
		}
		atoms := binary.LittleEndian.Uint64(value)
		if atoms > uint64(poolsettle.MaxMoney)-total {
			return walletParsedPayout{}, false
		}
		total += atoms
		script := c.take(c.count())
		if c.bad {
			return walletParsedPayout{}, false
		}
		if uint64(outputIndex) == i {
			result.atoms, result.script = int64(atoms), script
		}
	}
	strippedEnd := c.pos
	if witness {
		anyWitness := false
		for i := uint64(0); i < inputs; i++ {
			items := c.count()
			if c.bad || items > uint64(len(raw)-c.pos) {
				return walletParsedPayout{}, false
			}
			anyWitness = anyWitness || items != 0
			for j := uint64(0); j < items; j++ {
				c.take(c.count())
				if c.bad {
					return walletParsedPayout{}, false
				}
			}
		}
		if !anyWitness {
			return walletParsedPayout{}, false
		}
	}
	lockTime := c.take(4)
	if c.bad || c.pos != len(raw) {
		return walletParsedPayout{}, false
	}
	h := sha256.New()
	_, _ = h.Write(raw[:4])
	_, _ = h.Write(raw[strippedStart:strippedEnd])
	_, _ = h.Write(lockTime)
	sum := sha256.Sum256(h.Sum(nil))
	for i := 0; i < len(sum)/2; i++ {
		sum[i], sum[len(sum)-1-i] = sum[len(sum)-1-i], sum[i]
	}
	result.txid = hex.EncodeToString(sum[:])
	result.inputs, result.outputs, result.witness = inputs, outputs, witness
	return result, true
}
