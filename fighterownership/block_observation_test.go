// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestOwnershipObservedBlockExactMerkleMembership(t *testing.T) {
	// Three independently assembled legacy transaction byte strings. No
	// production Merkle helper or expected-output file creates this tree.
	tx := func(index string) []byte {
		raw, e := hex.DecodeString("0100000001" + strings.Repeat("11", 32) + index + "000000" + "00ffffffff" + "01" + "0100000000000000" + "0151" + "00000000")
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	a, b, c := tx("01"), tx("02"), tx("03")
	hash := func(raw []byte) [32]byte { first := sha256.Sum256(raw); return sha256.Sum256(first[:]) }
	join := func(a, b [32]byte) [32]byte { raw := append(append([]byte{}, a[:]...), b[:]...); return hash(raw) }
	ha, hb, hc := hash(a), hash(b), hash(c)
	root := join(join(ha, hb), join(hc, hc))
	header := make([]byte, 80)
	copy(header[36:68], root[:])
	h := ChainHeader{Height: 1, Hash: strings.Repeat("11", 32), Raw: header}
	block := append(append(append(append([]byte{}, header...), 3), a...), b...)
	block = append(block, c...)
	display := hb
	for i, j := 0, 31; i < j; i, j = i+1, j-1 {
		display[i], display[j] = display[j], display[i]
	}
	id := hex.EncodeToString(display[:])
	if pos, e := VerifyBlockTransaction(context.Background(), block, h, id, b); e != nil || pos != 1 {
		t.Fatal("valid three-leaf proof", pos, e)
	}
	// Padding a 3-leaf tree with a duplicate fourth leaf preserves its root.
	// Duplicate transaction IDs must still reject the mutated body.
	duplicate := append([]byte{}, block...)
	duplicate[80] = 4
	duplicate = append(duplicate, c...)
	changed := append([]byte{}, block...)
	changed[len(changed)-1] ^= 1
	badHeader := append([]byte{}, block...)
	badHeader[36] ^= 1
	claimed := append(append([]byte{}, header...), 0xfe, 0xff, 0xff, 0xff, 0xff)
	for name, raw := range map[string][]byte{"duplicate-padding": duplicate, "changed-body": changed, "changed-header": badHeader, "claimed-count": claimed, "trailing": append(append([]byte{}, block...), 0)} {
		t.Run(name, func(t *testing.T) {
			if _, e := VerifyBlockTransaction(context.Background(), raw, h, id, b); e == nil {
				t.Fatal("invalid membership accepted")
			}
		})
	}
	if _, e := VerifyBlockTransaction(context.Background(), block, h, id, a); !errors.Is(e, ErrConflict) {
		t.Fatal("different saved bytes", e)
	}
	if _, e := VerifyBlockTransaction(context.Background(), block, h, strings.Repeat("22", 32), a); !errors.Is(e, ErrNotFound) {
		t.Fatal("absent exact ID", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := VerifyBlockTransaction(ctx, block, h, id, b); !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled block", e)
	}
}
