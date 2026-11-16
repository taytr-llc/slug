package blockchain

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestReadBitcoinFixtureBlocks(t *testing.T) {
	var serialized bytes.Buffer
	if err := Block100000.Serialize(&serialized); err != nil {
		t.Fatal(err)
	}
	raw := serialized.Bytes()
	frame := func(magic, length uint32, payload []byte) []byte {
		result := make([]byte, 8+len(payload))
		binary.LittleEndian.PutUint32(result[:4], magic)
		binary.LittleEndian.PutUint32(result[4:8], length)
		copy(result[8:], payload)
		return result
	}
	valid := frame(bitcoinFixtureMagic, uint32(len(raw)), raw)
	blocks, err := readBitcoinFixtureBlocks(bytes.NewReader(valid))
	if err != nil || len(blocks) != 1 {
		t.Fatalf("retained Bitcoin fixture did not load: blocks=%d err=%v", len(blocks), err)
	}

	tests := map[string][]byte{
		"empty":             nil,
		"partial magic":     valid[:2],
		"wrong network":     frame(uint32(wire.MainNet), uint32(len(raw)), raw),
		"missing length":    valid[:4],
		"partial length":    valid[:6],
		"zero length":       frame(bitcoinFixtureMagic, 0, nil),
		"excessive length":  frame(bitcoinFixtureMagic, wire.MaxBlockPayload+1, nil),
		"truncated payload": valid[:len(valid)-1],
		"trailing garbage":  append(append([]byte(nil), valid...), 1),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := readBitcoinFixtureBlocks(bytes.NewReader(data)); err == nil {
				t.Fatal("malformed fixture was accepted")
			}
		})
	}
}
