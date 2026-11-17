// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterbout

import (
	"context"
	"math"

	"github.com/taytr-llc/slug/fightercondition"
)

type strike struct {
	name   string
	damage int64
	power  bool
}

// Arrays are returned by value: no mutable global model state or caller table.
func strikes() [5]strike {
	return [5]strike{{"jab", 1200000, false}, {"cross", 2300000, true},
		{"hook", 2800000, true}, {"uppercut", 3300000, true}, {"body", 1800000, false}}
}
func styleWeights(style string) [5]int64 {
	switch style {
	case "slugger":
		return [5]int64{25, 25, 25, 15, 10}
	case "outfighter":
		return [5]int64{50, 20, 10, 5, 15}
	case "boxer_puncher":
		return [5]int64{35, 25, 20, 10, 10}
	case "pressure":
		return [5]int64{30, 20, 25, 10, 15}
	default: // Entire manifest validation has already required a known style.
		return [5]int64{40, 25, 15, 8, 12}
	}
}

type bout struct {
	input   Input
	math    arithmetic
	random  draws
	initial [2]Initial
	states  [2]State
}

func (b *bout) err() error {
	if b.math.err != nil {
		return b.math.err
	}
	return b.random.err
}

func (b *bout) initialize(conditions [2]fightercondition.Snapshot) {
	forms := [2]int64{b.random.uniform(0, 200001) - 100000,
		b.random.uniform(1, 200001) - 100000}
	jitters := [2]int64{920000 + b.random.uniform(2, 160001),
		920000 + b.random.uniform(3, 160001)}
	m := &b.math
	for i, f := range [2]FighterInput{b.input.A, b.input.B} {
		a := conditions[i].AgedAttributesTenths
		factor := m.add(Scale, forms[i])
		effective := AttributesQ{
			Power:   m.mul(m.product(int64(a.Power), 100000), factor),
			Speed:   m.mul(m.product(int64(a.Speed), 100000), factor),
			Chin:    m.mul(m.product(int64(a.Chin), 100000), factor),
			Stamina: m.mul(m.product(int64(a.Stamina), 100000), factor),
			Skill:   m.mul(m.product(int64(a.Skill), 100000), factor),
		}
		effective.Skill = m.add(effective.Skill,
			m.product(int64(f.ProXP)+int64(f.TrainingSkill), Scale))
		b.initial[i] = Initial{FormQ: forms[i], JitterQ: jitters[i], Effective: effective,
			MaxStaminaQ:  m.add(60*Scale, m.mul(effective.Stamina, 1400000)),
			KOThresholdQ: m.mul(m.add(70*Scale, m.mul(effective.Chin, 450000)), jitters[i])}
		b.states[i].StaminaQ = b.initial[i].MaxStaminaQ
	}
}

func physicalTerms(att, def FighterInput) (int64, int64) {
	a, d := att.Manifest.Physical, def.Manifest.Physical
	// Regenerated physical values are small positive inches/pounds. These
	// products have the exact contract bounds [-30000,30000] and [960000,1040000].
	rangeHit := 2500*clamp(int64(a.Reach)-int64(d.Reach), -8, 8) +
		1250*clamp(int64(a.Height)-int64(d.Height), -8, 8)
	mass := Scale + 1000*clamp(int64(a.Weight)-int64(d.Weight), -40, 40)
	return rangeHit, mass
}

func (b *bout) step(number uint32) Tick {
	m, rng := &b.math, &b.random
	fA := m.div(b.states[0].StaminaQ, b.initial[0].MaxStaminaQ)
	fB := m.div(b.states[1].StaminaQ, b.initial[1].MaxStaminaQ)
	wA := m.mul(b.initial[0].Effective.Speed, m.add(450000, m.mul(550000, fA)))
	wB := m.mul(b.initial[1].Effective.Speed, m.add(450000, m.mul(550000, fB)))
	pA := m.div(wA, m.add(wA, wB))
	actor := 1
	if rng.chance(pA) {
		actor = 0
	}
	fighters := [2]FighterInput{b.input.A, b.input.B}
	ticket := rng.uniform(4, 100)
	weights := styleWeights(fighters[actor].Manifest.Style)
	index, total := 0, int64(0)
	for i, weight := range weights {
		total += weight
		if ticket < total {
			index = i
			break
		}
	}
	s := strikes()[index]
	tick := Tick{Number: number, Actor: uint32(actor), Strike: s.name}
	att, def := &b.states[actor], &b.states[1-actor]
	initAtt, initDef := b.initial[actor], b.initial[1-actor]
	att.Thrown++ // At most one per tick, at most 180 ticks.
	cost := int64(800000)
	if s.power {
		cost = 1150000
	}
	att.StaminaQ = max(0, m.sub(att.StaminaQ, cost))
	rangeHit, massDamage := physicalTerms(fighters[actor], fighters[1-actor])
	p := m.add(300000, m.mul(m.sub(initAtt.Effective.Skill, initDef.Effective.Speed), 2200))
	if fighters[actor].Manifest.Physical.Stance != fighters[1-actor].Manifest.Physical.Stance {
		p = m.add(p, 20000)
	}
	p = m.add(p, rangeHit)
	p = m.mul(p, m.add(720000, m.mul(280000, m.div(att.StaminaQ, initAtt.MaxStaminaQ))))
	p = m.add(p, m.mul(max(0, m.sub(550000, m.div(def.StaminaQ, initDef.MaxStaminaQ))), 100000))
	p = clamp(p, 60000, 780000)
	tick.Hit = rng.chance(p)
	if !tick.Hit {
		att.ControlQ = m.add(att.ControlQ, 500000)
		tick.States = b.states
		return tick
	}
	att.Landed++
	damageFactor := m.add(600000, rng.uniform(4, 850001))
	damage := m.mul(m.mul(m.mul(s.damage,
		m.add(650000, m.round(initAtt.Effective.Power, 100))), damageFactor), massDamage)
	tick.DamageQ = damage
	if s.name == "jab" {
		att.ControlQ = m.add(att.ControlQ, 500000)
	}
	if s.name == "body" {
		def.StaminaQ = max(0, m.sub(def.StaminaQ, m.mul(damage, 900000)))
	}
	tick.Significant = s.power || damage >= 1600000
	if tick.Significant {
		att.Significant++
	}
	att.DamageDealtQ = m.add(att.DamageDealtQ, damage)
	def.DamageTakenQ = m.add(def.DamageTakenQ, damage)
	if tick.Significant {
		b.significantShot(actor, &tick)
	}
	tick.States = b.states
	return tick
}

func (b *bout) significantShot(actor int, tick *Tick) {
	m, rng := &b.math, &b.random
	att, def := &b.states[actor], &b.states[1-actor]
	initDef := b.initial[1-actor]
	v := m.div(def.DamageTakenQ, initDef.KOThresholdQ)
	pKD := m.sub(m.add(m.add(8000, m.mul(max(0, m.sub(tick.DamageQ, 3000000)), 18000)),
		m.mul(m.mul(v, v), 220000)), m.mul(initDef.Effective.Chin, 120))
	pKD = clamp(pKD, 0, 450000)
	tick.Knockdown = rng.chance(pKD)
	if tick.Knockdown {
		def.RoundKnockdowns++
		att.Knockdowns++
		att.ControlQ = m.add(att.ControlQ, 3000000)
		def.DamageTakenQ = m.add(def.DamageTakenQ, 6000000)
		att.DamageDealtQ = m.add(att.DamageDealtQ, 6000000)
		if def.RoundKnockdowns == 3 {
			tick.Stoppage = "tko_three_knockdowns"
			return
		}
	}
	v = m.div(def.DamageTakenQ, initDef.KOThresholdQ)
	if v >= Scale {
		tick.Stoppage = "ko"
		return
	}
	if v >= 850000 {
		pRef := m.add(100000, m.mul(m.sub(v, 850000), 1200000))
		if rng.chance(pRef) {
			tick.Stoppage = "tko_referee"
		}
	}
}

func (b *bout) recoverRound() Recovery {
	m := &b.math
	for i := range b.states {
		s, initial := &b.states[i], b.initial[i]
		recovery := m.mul(initial.MaxStaminaQ,
			m.add(100000, m.mul(m.round(initial.Effective.Stamina, 100), 100000)))
		s.StaminaQ = min(initial.MaxStaminaQ, m.add(s.StaminaQ, recovery))
		s.DamageTakenQ = m.mul(s.DamageTakenQ, 970000)
	}
	r := Recovery{States: b.states}
	for i := range b.states {
		if m.div(b.states[i].DamageTakenQ, b.initial[i].KOThresholdQ) >= 970000 {
			r.CornerStopped[i] = b.random.chance(350000)
		}
	}
	return r
}

func scoreState(state State) (Score, error) {
	m := arithmetic{}
	damage := m.round(state.DamageDealtQ, 100000)
	knockdown := m.product(100, int64(state.Knockdowns))
	accuracy := int64(0)
	if state.Thrown != 0 {
		accuracy = m.round(m.product(250, int64(state.Landed)), int64(state.Thrown))
	}
	volume := m.product(6, int64(state.Significant))
	control := m.round(m.product(7, state.ControlQ), 2*Scale)
	total := m.add(m.add(m.add(m.add(damage, knockdown), accuracy), volume), control)
	if m.err != nil {
		return Score{}, m.err
	}
	for _, v := range [6]int64{damage, knockdown, accuracy, volume, control, total} {
		if v < 0 || v > math.MaxUint32 {
			return Score{}, ErrArithmetic
		}
	}
	return Score{DamageTenths: uint32(damage), KnockdownTenths: uint32(knockdown),
		AccuracyTenths: uint32(accuracy), VolumeTenths: uint32(volume),
		ControlTenths: uint32(control), TotalTenths: uint32(total)}, nil
}

func cornerDecision(stopped [2]bool) (string, int) {
	if stopped[0] && stopped[1] {
		return "double_corner_stoppage", -1
	}
	if stopped[0] {
		return "tko_corner", 1
	}
	if stopped[1] {
		return "tko_corner", 0
	}
	return "", -1
}

func scoreDecision(a, b uint32) (string, int) {
	if a > b {
		return "decision", 0
	}
	if b > a {
		return "decision", 1
	}
	return "decision_draw", -1
}

// Simulate runs only the frozen standard model. It neither selects an official
// seed nor records a fight/award, and returns no partial transcript on failure.
func Simulate(ctx context.Context, in Input) (Result, error) {
	if ctx == nil {
		return Result{}, ErrInput
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	conditions, err := validateInput(in)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	id, err := inputID(in)
	if err != nil {
		return Result{}, err
	}
	b := bout{input: in, random: newDraws(ctx, id)}
	b.initialize(conditions)
	if err := b.err(); err != nil {
		return Result{}, err
	}
	result := Result{Version: Version1, EngineID: EngineID, InputID: id,
		Initial: b.initial, Rounds: make([]Round, 0, Rounds)}
	ids := [2]string{in.A.ProspectID, in.B.ProspectID}
	terminal := false
	for number := uint32(1); number <= Rounds; number++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		for i := range b.states {
			b.states[i].RoundKnockdowns = 0
		}
		round := Round{Number: number, Ticks: make([]Tick, 0, TicksPerRound)}
		for tickNumber := uint32(1); tickNumber <= TicksPerRound; tickNumber++ {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			tick := b.step(tickNumber)
			if err := b.err(); err != nil {
				return Result{}, err
			}
			round.Ticks = append(round.Ticks, tick)
			result.EndRound, result.EndTick = number, tickNumber
			if tick.Stoppage != "" {
				result.Winner, result.Method = ids[tick.Actor], tick.Stoppage
				terminal = true
				break
			}
		}
		round.End = b.states
		if !terminal && number < Rounds {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			recovery := b.recoverRound()
			if err := b.err(); err != nil {
				return Result{}, err
			}
			round.Recovery = &recovery
			method, winner := cornerDecision(recovery.CornerStopped)
			if method != "" {
				terminal = true
				result.Method = method
				if winner >= 0 {
					result.Winner = ids[winner]
				}
			}
		}
		result.Rounds = append(result.Rounds, round)
		if terminal {
			break
		}
	}
	result.Final, result.DrawCounts = b.states, b.random.counts()
	for i, state := range result.Final {
		result.Scores[i], err = scoreState(state)
		if err != nil {
			return Result{}, err
		}
	}
	if !terminal {
		method, winner := scoreDecision(result.Scores[0].TotalTenths, result.Scores[1].TotalTenths)
		result.Method = method
		if winner >= 0 {
			result.Winner = ids[winner]
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	// The bounds are proved by the model and enforced at the publication edge.
	if _, err := canonicalJSON(result, MaxResultBytes); err != nil {
		return Result{}, ErrResult
	}
	return result, nil
}
