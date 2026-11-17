// Copyright (c) 2026 The SLUG developers. ISC license.

// Package fighterbout implements a deterministic candidate fight model. Its
// supplied identities, birth, bonuses and seed grant no admission, award,
// ownership, eligibility, official result or randomness-provenance authority.
package fighterbout

import (
	"errors"
	"github.com/taytr-llc/slug/fightermanifest"
)

const (
	Version1           uint32 = 1
	EngineID                  = "slug-fighter-bout-candidate-v1"
	Scale              int64  = 1000000
	Rounds             uint32 = 3
	TicksPerRound      uint32 = 60
	MaxInputBytes             = 32768
	MaxResultBytes            = 196608
	MaxJSONDepth              = 8
	MaxRawWords        uint32 = 65536
	MaxUniformAttempts uint32 = 256
	maxDateMS          int64  = 8640000000000000
	inputDomain               = "SLUG fighter bout input\x00v1\x00"
	resultDomain              = "SLUG fighter bout result\x00v1\x00"
	randomDomain              = "SLUG fighter bout random\x00v1\x00"
)

var (
	ErrInput      = errors.New("invalid fighter bout input")
	ErrResult     = errors.New("invalid fighter bout result")
	ErrStream     = errors.New("fighter bout stream exhausted")
	ErrArithmetic = errors.New("fighter bout arithmetic out of range")
)

type FighterInput struct {
	ProspectID    string                   `json:"prospect_id"`
	Manifest      fightermanifest.Manifest `json:"manifest"`
	BornAtMS      int64                    `json:"born_at_ms"`
	ProXP         uint32                   `json:"pro_xp"`
	TrainingSkill uint32                   `json:"training_skill"`
}
type Input struct {
	Version  uint32       `json:"version"`
	EngineID string       `json:"engine_id"`
	Mode     string       `json:"mode"`
	SeedKind string       `json:"seed_kind"`
	AsOfMS   int64        `json:"as_of_ms"`
	Seed     string       `json:"seed"`
	A        FighterInput `json:"a"`
	B        FighterInput `json:"b"`
}

type AttributesQ struct {
	Power   int64 `json:"power"`
	Speed   int64 `json:"speed"`
	Chin    int64 `json:"chin"`
	Stamina int64 `json:"stamina"`
	Skill   int64 `json:"skill"`
}
type Initial struct {
	FormQ        int64       `json:"form_q"`
	JitterQ      int64       `json:"jitter_q"`
	Effective    AttributesQ `json:"effective"`
	MaxStaminaQ  int64       `json:"max_stamina_q"`
	KOThresholdQ int64       `json:"ko_threshold_q"`
}
type State struct {
	StaminaQ        int64  `json:"stamina_q"`
	DamageTakenQ    int64  `json:"damage_taken_q"`
	Thrown          uint32 `json:"thrown"`
	Landed          uint32 `json:"landed"`
	Significant     uint32 `json:"significant"`
	DamageDealtQ    int64  `json:"damage_dealt_q"`
	Knockdowns      uint32 `json:"knockdowns"`
	ControlQ        int64  `json:"control_q"`
	RoundKnockdowns uint32 `json:"round_knockdowns"`
}
type Tick struct {
	Number      uint32   `json:"number"`
	Actor       uint32   `json:"actor"` // 0=A,1=B
	Strike      string   `json:"strike"`
	Hit         bool     `json:"hit"`
	DamageQ     int64    `json:"damage_q"`
	Significant bool     `json:"significant"`
	Knockdown   bool     `json:"knockdown"`
	Stoppage    string   `json:"stoppage"` // empty or one in-tick finish method
	States      [2]State `json:"states"`
}
type Recovery struct {
	States        [2]State `json:"states"`
	CornerStopped [2]bool  `json:"corner_stopped"`
}
type Round struct {
	Number   uint32    `json:"number"`
	Ticks    []Tick    `json:"ticks"`
	End      [2]State  `json:"end"` // after ticks, before recovery
	Recovery *Recovery `json:"recovery"`
}
type Score struct {
	DamageTenths    uint32 `json:"damage_tenths"`
	KnockdownTenths uint32 `json:"knockdown_tenths"`
	AccuracyTenths  uint32 `json:"accuracy_tenths"`
	VolumeTenths    uint32 `json:"volume_tenths"`
	ControlTenths   uint32 `json:"control_tenths"`
	TotalTenths     uint32 `json:"total_tenths"`
}
type DrawCounts struct {
	FormA  uint32 `json:"form_a"`
	FormB  uint32 `json:"form_b"`
	ChinA  uint32 `json:"chin_a"`
	ChinB  uint32 `json:"chin_b"`
	Combat uint32 `json:"combat"`
}
type Result struct {
	Version    uint32     `json:"version"`
	EngineID   string     `json:"engine_id"`
	InputID    string     `json:"input_id"`
	Initial    [2]Initial `json:"initial"`
	Rounds     []Round    `json:"rounds"`
	Final      [2]State   `json:"final"`
	Winner     string     `json:"winner"`
	Method     string     `json:"method"`
	EndRound   uint32     `json:"end_round"`
	EndTick    uint32     `json:"end_tick"`
	Scores     [2]Score   `json:"scores"`
	DrawCounts DrawCounts `json:"draw_counts"`
}
