// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterbout

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
)

func testInput(t *testing.T) Input {
	t.Helper()
	m, err := fightermanifest.Generate(1, [32]byte{})
	if err != nil {
		t.Fatal(err)
	}
	a := FighterInput{ProspectID: strings.Repeat("0", 64), Manifest: m, BornAtMS: 1700000000000}
	b := a
	b.ProspectID = strings.Repeat("f", 64)
	in, err := NewInput(a.BornAtMS, strings.Repeat("0", 64), a, b)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func fixedWords(t *testing.T, words ...uint64) (*draws, *int) {
	t.Helper()
	count := 0
	d := &draws{nextWord: func(stream int) (uint64, error) {
		if stream != 4 || count >= len(words) {
			t.Fatalf("unexpected draw %d from stream %d", count, stream)
		}
		word := words[count]
		count++
		return word, nil
	}}
	return d, &count
}

func TestSignificantFinishPrecedence(t *testing.T) {
	const acceptZero, acceptHigh = uint64(10000000), uint64(10999999)
	for _, tc := range []struct {
		name   string
		hurt   int64
		round  uint32
		words  []uint64
		method string
		kd     bool
	}{
		{"third knockdown before KO", 100 * Scale, 2, []uint64{acceptZero}, "tko_three_knockdowns", true},
		{"KO before referee", 100 * Scale, 0, []uint64{acceptHigh}, "ko", false},
		{"referee true", 90 * Scale, 0, []uint64{acceptHigh, acceptZero}, "tko_referee", false},
		{"referee false", 90 * Scale, 0, []uint64{acceptHigh, acceptHigh}, "", false},
		{"below referee threshold", 85*Scale - 100, 0, []uint64{acceptHigh}, "", false},
		{"at referee threshold", 85 * Scale, 0, []uint64{acceptHigh, acceptZero}, "tko_referee", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rng, calls := fixedWords(t, tc.words...)
			b := bout{random: *rng}
			b.initial[1] = Initial{KOThresholdQ: 100 * Scale, Effective: AttributesQ{Chin: 10 * Scale}}
			b.states[1] = State{DamageTakenQ: tc.hurt, RoundKnockdowns: tc.round}
			tick := Tick{DamageQ: 4 * Scale, Significant: true}
			b.significantShot(0, &tick)
			if b.err() != nil || tick.Stoppage != tc.method || tick.Knockdown != tc.kd || *calls != len(tc.words) {
				t.Fatalf("finish=%s kd=%v calls=%d err=%v", tick.Stoppage, tick.Knockdown, *calls, b.err())
			}
			if tc.kd && (b.states[0].Knockdowns != 1 || b.states[0].ControlQ != 3*Scale || b.states[1].DamageTakenQ != tc.hurt+6*Scale) {
				t.Fatal("knockdown state not recorded once")
			}
		})
	}
}

func TestMissAndNonsignificantShotsDrawNoFinishWords(t *testing.T) {
	const low, high = uint64(10000000), uint64(10999999)
	for _, tc := range []struct {
		name    string
		words   []uint64
		hit     bool
		strike  string
		damage  int64
		control int64
		defCost int64
	}{
		{"miss", []uint64{low, 1000, high}, false, "jab", 0, 500000, 0},
		{"small jab", []uint64{low, 1000, low, 8500010}, true, "jab", 828000, 500000, 0},
		{"small body", []uint64{low, 1095, low, 8500010}, true, "body", 1242000, 0, 1117800},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rng, count := fixedWords(t, tc.words...)
			b := bout{random: *rng}
			for i := range b.initial {
				b.initial[i] = Initial{Effective: AttributesQ{Power: 50 * Scale, Speed: 50 * Scale,
					Skill: 50 * Scale, Chin: 50 * Scale}, MaxStaminaQ: 100 * Scale, KOThresholdQ: 100 * Scale}
				b.states[i].StaminaQ = 100 * Scale
			}
			b.input.A.Manifest.Style, b.input.B.Manifest.Style = "boxer", "boxer"
			tick := b.step(1)
			if b.err() != nil || *count != len(tc.words) || tick.Actor != 0 || tick.Hit != tc.hit ||
				tick.Strike != tc.strike || tick.DamageQ != tc.damage || tick.Significant || tick.Knockdown || tick.Stoppage != "" {
				t.Fatalf("private step %+v; draws=%d error=%v", tick, *count, b.err())
			}
			if b.states[0].ControlQ != tc.control || b.states[0].StaminaQ != 100*Scale-800000 ||
				b.states[1].StaminaQ != 100*Scale-tc.defCost || b.states[0].DamageDealtQ != tc.damage || b.states[1].DamageTakenQ != tc.damage {
				t.Fatal("small shot state")
			}
		})
	}
}

func TestInitializationFormBonusOrderAndDrawEndpoints(t *testing.T) {
	order := []int{}
	b := bout{input: Input{A: FighterInput{ProXP: 15, TrainingSkill: 5}, B: FighterInput{ProXP: 15, TrainingSkill: 5}}}
	b.random.nextWord = func(stream int) (uint64, error) {
		order = append(order, stream)
		switch stream {
		case 0:
			return 2000010, nil // form -.10
		case 1:
			return 2200010, nil // form +.10
		case 2:
			return 1600010, nil // jitter .92
		case 3:
			return 1760010, nil // jitter 1.08
		default:
			t.Fatal("combat draw during initialization")
			return 0, ErrStream
		}
	}
	attrs := fightercondition.AttributesTenths{Power: 1000, Speed: 1000, Chin: 1000, Stamina: 1000, Skill: 1000}
	c := fightercondition.Snapshot{AgedAttributesTenths: attrs, LeagueAttributesTenths: attrs}
	c.LeagueAttributesTenths.Skill = 1150 // Must not feed back into form or XP.
	b.initialize([2]fightercondition.Snapshot{c, c})
	if b.err() != nil || !reflect.DeepEqual(order, []int{0, 1, 2, 3}) {
		t.Fatal("initial draw order", b.err())
	}
	if b.initial[0].FormQ != -100000 || b.initial[1].FormQ != 100000 ||
		b.initial[0].JitterQ != 920000 || b.initial[1].JitterQ != 1080000 ||
		b.initial[0].Effective.Skill != 110*Scale || b.initial[1].Effective.Skill != 130*Scale ||
		b.initial[0].Effective.Chin != 90*Scale || b.initial[1].Effective.Chin != 110*Scale {
		t.Fatal("form or bonus order, endpoint, or unsupported final clamp")
	}
}

func TestRecoveryBothCornersAndNoHealingOfDealt(t *testing.T) {
	rng, calls := fixedWords(t, 10000000, 10000000)
	b := bout{random: *rng}
	for i := range b.initial {
		b.initial[i] = Initial{KOThresholdQ: 100 * Scale, MaxStaminaQ: 100 * Scale,
			Effective: AttributesQ{Stamina: 100 * Scale}}
		b.states[i] = State{DamageTakenQ: 100 * Scale, DamageDealtQ: 123 * Scale, StaminaQ: 90 * Scale}
	}
	r := b.recoverRound()
	if b.err() != nil || r.CornerStopped != [2]bool{true, true} || *calls != 2 {
		t.Fatalf("both corners %+v err=%v", r, b.err())
	}
	for _, state := range r.States {
		if state.DamageTakenQ != 97*Scale || state.DamageDealtQ != 123*Scale || state.StaminaQ != 100*Scale {
			t.Fatal("recovery state did not use independent capped stamina and taken-only healing")
		}
	}
}

func TestScoreUsesRoundedPublishedParts(t *testing.T) {
	// Damage 0.25 rounds to 0.2; accuracy 250/3 rounds to 83;
	// control 7*1/2 is an odd half and rounds to four tenths.
	s, err := scoreState(State{DamageDealtQ: 250000, Knockdowns: 2,
		Thrown: 3, Landed: 1, Significant: 1, ControlQ: Scale})
	want := Score{DamageTenths: 2, KnockdownTenths: 200, AccuracyTenths: 83,
		VolumeTenths: 6, ControlTenths: 4, TotalTenths: 295}
	if err != nil || s != want {
		t.Fatalf("score %+v,%v; want %+v", s, err, want)
	}
	if zero, err := scoreState(State{}); err != nil || zero != (Score{}) {
		t.Fatal("empty accuracy denominator")
	}
}

func TestPhysicalClampAndMixtures(t *testing.T) {
	a, b := FighterInput{}, FighterInput{}
	a.Manifest.Physical = fightermanifest.Physical{Height: 80, Reach: 84, Weight: 268}
	b.Manifest.Physical = fightermanifest.Physical{Height: 62, Reach: 60, Weight: 128}
	r, m := physicalTerms(a, b)
	if r != 30000 || m != 1040000 {
		t.Fatal("upper physical clamps")
	}
	r, m = physicalTerms(b, a)
	if r != -30000 || m != 960000 {
		t.Fatal("lower physical clamps")
	}
	for _, style := range []string{"slugger", "outfighter", "boxer_puncher", "pressure", "boxer"} {
		var total int64
		for _, v := range styleWeights(style) {
			total += v
		}
		if total != 100 {
			t.Fatal("style mixture does not sum to 100")
		}
	}
}

func TestSimulationTranscriptAndOwnership(t *testing.T) {
	in := testInput(t)
	before := in
	r, err := Simulate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if in != before || len(r.Rounds) < 1 || len(r.Rounds) > 3 {
		t.Fatal("input mutation or round bound")
	}
	for ri, round := range r.Rounds {
		if round.Number != uint32(ri+1) || len(round.Ticks) == 0 || len(round.Ticks) > 60 {
			t.Fatal("round continuity")
		}
		for ti, tick := range round.Ticks {
			if tick.Number != uint32(ti+1) || tick.Actor > 1 || !tick.Hit && (tick.DamageQ != 0 || tick.Significant || tick.Knockdown || tick.Stoppage != "") {
				t.Fatal("tick continuity or miss side effect")
			}
			for i, s := range tick.States {
				if s.StaminaQ < 0 || s.StaminaQ > r.Initial[i].MaxStaminaQ ||
					s.Landed > s.Thrown || s.Significant > s.Landed || s.RoundKnockdowns > 3 {
					t.Fatal("bounded aggregate state")
				}
			}
		}
		if round.End != round.Ticks[len(round.Ticks)-1].States {
			t.Fatal("round end drift")
		}
		if round.Number == Rounds && round.Recovery != nil {
			t.Fatal("post-final-bell recovery")
		}
	}
	last := r.Rounds[len(r.Rounds)-1]
	final := last.End
	if last.Recovery != nil {
		final = last.Recovery.States
	}
	if r.Final != final || r.EndRound != last.Number || r.EndTick != uint32(len(last.Ticks)) {
		t.Fatal("final state drift")
	}
	want, err := ValidateResult(context.Background(), in, r)
	if err != nil || !reflect.DeepEqual(want, r) {
		t.Fatal("own simulation did not validate")
	}
	want.Rounds[0].Ticks[0].Number = 500
	if r.Rounds[0].Ticks[0].Number != 1 {
		t.Fatal("returned validation aliases supplied transcript")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := Simulate(context.Background(), in)
			if err != nil || !reflect.DeepEqual(got, r) {
				t.Error("concurrent simulation differs")
			}
		}()
	}
	wg.Wait()
}

type cancelOnCheck struct {
	context.Context
	remaining int
}

func (c *cancelOnCheck) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}

func TestCancellationReturnsNoTranscript(t *testing.T) {
	in := testInput(t)
	for _, n := range []int{1, 2, 3, 10, 20} {
		ctx := &cancelOnCheck{Context: context.Background(), remaining: n}
		r, err := Simulate(ctx, in)
		if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(r, Result{}) {
			t.Fatalf("check %d returned partial result or wrong error %v", n, err)
		}
	}
}
