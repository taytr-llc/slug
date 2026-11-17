// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterbout

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
)

type wordBudget struct {
	ctx   context.Context
	count uint32
}

type randomStream struct {
	id      [32]byte
	label   string
	counter uint64
	budget  *wordBudget
}

func (s *randomStream) next() (uint64, error) {
	if err := s.budget.ctx.Err(); err != nil {
		return 0, err
	}
	if s.budget.count >= MaxRawWords || s.counter == math.MaxUint64 {
		return 0, ErrStream
	}
	h := sha256.New()
	h.Write([]byte(randomDomain))
	h.Write(s.id[:])
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(s.label)))
	h.Write(size[:])
	h.Write([]byte(s.label))
	var counter [8]byte
	binary.LittleEndian.PutUint64(counter[:], s.counter)
	h.Write(counter[:])
	s.counter++
	s.budget.count++
	return binary.BigEndian.Uint64(h.Sum(nil)[:8]), nil
}

// uniform's private word seam permits independent rejection-boundary tests.
// All public simulation streams are constructed internally from InputID.
func uniform(n uint64, next func() (uint64, error)) (uint64, error) {
	if n == 0 {
		return 0, ErrStream
	}
	threshold := -n % n
	for i := uint32(0); i < MaxUniformAttempts; i++ {
		word, err := next()
		if err != nil {
			return 0, err
		}
		if word >= threshold {
			return word % n, nil
		}
	}
	return 0, ErrStream
}

type draws struct {
	streams [5]randomStream
	err     error
	// Only private primitive tests supply words. Public construction always
	// leaves this nil and binds every word to InputID with a shared budget.
	nextWord func(stream int) (uint64, error)
}

func newDraws(ctx context.Context, id string) draws {
	var key [32]byte
	raw, _ := hex.DecodeString(id) // Only the internally computed InputID.
	copy(key[:], raw)
	budget := &wordBudget{ctx: ctx}
	var d draws
	for i, label := range [5]string{"form-a", "form-b", "chin-a", "chin-b", "combat"} {
		d.streams[i] = randomStream{id: key, label: label, budget: budget}
	}
	return d
}
func (d *draws) uniform(stream int, n uint64) int64 {
	if d.err != nil {
		return 0
	}
	next := d.streams[stream].next
	if d.nextWord != nil {
		next = func() (uint64, error) { return d.nextWord(stream) }
	}
	value, err := uniform(n, next)
	d.err = err
	return int64(value)
}
func (d *draws) chance(p int64) bool {
	if p < 0 || p > Scale {
		d.err = ErrArithmetic
		return false
	}
	return d.uniform(4, uint64(Scale)) < p
}
func (d *draws) counts() DrawCounts {
	return DrawCounts{FormA: uint32(d.streams[0].counter), FormB: uint32(d.streams[1].counter),
		ChinA: uint32(d.streams[2].counter), ChinB: uint32(d.streams[3].counter), Combat: uint32(d.streams[4].counter)}
}
