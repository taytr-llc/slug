// Copyright (c) 2026 The SLUG developers. ISC license.

// Package fightercareer implements pure D41/D42 accounting arithmetic. It does
// not authenticate results, deduplicate awards or establish ranking/title status.
package fightercareer

import (
	"errors"
	"math"
)

var (
	ErrInvalid  = errors.New("invalid career arithmetic input")
	ErrOverflow = errors.New("career arithmetic overflow")
)

// Delta describes one corner's contribution from one completed professional
// bout. XP is earned XP before the permanent cap, not the applied increment.
type Delta struct {
	Wins       uint32 `json:"wins"`
	Losses     uint32 `json:"losses"`
	Draws      uint32 `json:"draws"`
	XP         uint32 `json:"xp"`
	BasePoints uint32 `json:"base_points"`
}

// Totals contains counted history. BasePoints excludes opponent-quality points
// and is not a rolling ranking or a claim about the fighter's current position.
type Totals struct {
	Wins       uint32 `json:"wins"`
	Losses     uint32 `json:"losses"`
	Draws      uint32 `json:"draws"`
	XP         uint32 `json:"xp"`
	BasePoints uint32 `json:"base_points"`
}

// Award returns corner A then B. A trusted caller must separately authenticate
// the outcome, bout kind and finality before using these arithmetic values.
func Award(winner, method, kind string) ([2]Delta, error) {
	var xp uint32
	switch kind {
	case "standard":
		xp = 1
	case "title":
		xp = 2
	default:
		return [2]Delta{}, ErrInvalid
	}
	var points uint32
	switch method {
	case "decision_draw", "double_corner_stoppage":
		if winner != "draw" {
			return [2]Delta{}, ErrInvalid
		}
		d := Delta{Draws: 1, XP: xp, BasePoints: 4}
		return [2]Delta{d, d}, nil
	case "decision":
		points = 10
	case "ko", "tko_referee", "tko_three_knockdowns", "tko_corner":
		points = 14
	default:
		return [2]Delta{}, ErrInvalid
	}
	win := Delta{Wins: 1, XP: xp, BasePoints: points}
	loss := Delta{Losses: 1, XP: xp}
	switch winner {
	case "a":
		return [2]Delta{win, loss}, nil
	case "b":
		return [2]Delta{loss, win}, nil
	default:
		return [2]Delta{}, ErrInvalid
	}
}

func validDelta(d Delta) bool {
	if d.XP != 1 && d.XP != 2 {
		return false
	}
	switch {
	case d.Wins == 1 && d.Losses == 0 && d.Draws == 0:
		return d.BasePoints == 10 || d.BasePoints == 14
	case d.Wins == 0 && d.Losses == 1 && d.Draws == 0:
		return d.BasePoints == 0
	case d.Wins == 0 && d.Losses == 0 && d.Draws == 1:
		return d.BasePoints == 4
	default:
		return false
	}
}

// Apply adds one already-deduplicated event. It rejects invalid values and any
// counter overflow; reaching the XP cap still records the bout and its points.
func Apply(prior Totals, delta Delta) (Totals, error) {
	if prior.XP > 15 || !validDelta(delta) {
		return Totals{}, ErrInvalid
	}
	if prior.Wins > math.MaxUint32-delta.Wins ||
		prior.Losses > math.MaxUint32-delta.Losses ||
		prior.Draws > math.MaxUint32-delta.Draws ||
		prior.BasePoints > math.MaxUint32-delta.BasePoints {
		return Totals{}, ErrOverflow
	}
	return Totals{
		Wins: prior.Wins + delta.Wins, Losses: prior.Losses + delta.Losses,
		Draws: prior.Draws + delta.Draws, XP: min(15, prior.XP+delta.XP),
		BasePoints: prior.BasePoints + delta.BasePoints,
	}, nil
}

// QualityBonus computes the win-only bonus for an authenticated known rank.
// Zero is missing rank evidence and is refused, not treated as unranked.
func QualityBonus(opponentRank uint32) (uint32, error) {
	switch {
	case opponentRank == 0:
		return 0, ErrInvalid
	case opponentRank <= 5:
		return 6, nil
	case opponentRank <= 10:
		return 3, nil
	default:
		return 0, nil
	}
}
