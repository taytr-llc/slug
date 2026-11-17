// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercondition

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/fighter"
)

func testDNA(t *testing.T) fighter.DNA {
	t.Helper()
	dna, err := fighter.Generate(1, [32]byte{})
	if err != nil {
		t.Fatal(err)
	}
	return dna
}

func TestCompleteDNARefusal(t *testing.T) {
	base := testDNA(t)
	var visit func(reflect.Value, string)
	visit = func(v reflect.Value, prefix string) {
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if v.Field(i).Kind() == reflect.Struct {
				visit(v.Field(i), prefix+field.Name+".")
				continue
			}
			name := prefix + field.Name
			t.Run(name, func(t *testing.T) {
				dna := base
				value := reflect.ValueOf(&dna).Elem()
				for _, part := range strings.Split(name, ".") {
					value = value.FieldByName(part)
				}
				switch value.Kind() {
				case reflect.String:
					value.SetString(value.String() + "x")
				case reflect.Int:
					value.SetInt(value.Int() + 1)
				case reflect.Uint32:
					value.SetUint(value.Uint() + 1)
				default:
					t.Fatalf("untested new field: %s", value.Kind())
				}
				got, err := Project(1, dna, 1, 1, 0)
				if !errors.Is(err, ErrCondition) || got != (Snapshot{}) {
					t.Fatal("altered DNA accepted", got, err)
				}
			})
		}
	}
	visit(reflect.ValueOf(base), "")
	seed := [32]byte{}
	seed[0] = 0xab
	dna, err := fighter.Generate(1, seed)
	if err != nil {
		t.Fatal(err)
	}
	dna.Seed = strings.ToUpper(dna.Seed)
	if got, err := Project(1, dna, 1, 1, 0); err == nil || got != (Snapshot{}) {
		t.Fatal("uppercase canonical seed accepted")
	}
	dna = base
	dna.Attributes.Speed = math.MinInt
	if got, err := Project(1, dna, 1, 1, 0); err == nil || got != (Snapshot{}) {
		t.Fatal("negative attributes accepted")
	}
	dna = base
	dna.PrimeWeeks = math.MaxInt
	if got, err := Project(1, dna, 1, 1, 0); err == nil || got != (Snapshot{}) {
		t.Fatal("unbounded career accepted")
	}
}

func TestInputBoundsAndNoPartialSnapshot(t *testing.T) {
	dna := testDNA(t)
	for _, v := range []struct {
		version   uint32
		birth, at int64
		xp        int
	}{
		{0, 1, 1, 0}, {2, 1, 1, 0}, {math.MaxUint32, 1, 1, 0},
		{1, 0, 0, 0}, {1, -1, 1, 0}, {1, math.MinInt64, math.MaxInt64, 0},
		{1, 2, 1, 0}, {1, math.MaxInt64, math.MaxInt64 - 1, 0}, {1, 1, math.MinInt64, 0},
		{1, 1, 1, -1}, {1, 1, 1, 16}, {1, 1, 1, math.MinInt}, {1, 1, 1, math.MaxInt},
	} {
		got, err := Project(v.version, dna, v.birth, v.at, v.xp)
		if !errors.Is(err, ErrCondition) || got != (Snapshot{}) {
			t.Fatal("invalid input returned condition", v, got, err)
		}
	}
	for _, times := range [][2]int64{{1, 1}, {math.MaxInt64, math.MaxInt64}, {math.MaxInt64 - 1, math.MaxInt64}, {1, math.MaxInt64}} {
		got, err := Project(1, dna, times[0], times[1], 15)
		if err != nil {
			t.Fatal("valid extreme rejected", err)
		}
		if got.AgeWeeks != (times[1]-times[0])/604800000 {
			t.Fatal("elapsed overflow or narrowing", got)
		}
		if got.PrimeRemainingMS < 0 || got.CareerRemainingMS < 0 || got.AgeWeeks < 0 {
			t.Fatal("negative duration", got)
		}
	}
}

func TestExactCareerBoundaries(t *testing.T) {
	dna := testDNA(t)
	const born int64 = 1700000000000
	prime := int64(dna.PrimeWeeks) * 604800000
	end := int64(dna.PrimeWeeks+dna.DeclineWeeks) * 604800000
	for _, test := range []struct {
		offset            int64
		phase             string
		step, remainWeeks int
		remainMS          int64
	}{
		{0, "prime", 0, dna.PrimeWeeks, prime},
		{1, "prime", 0, dna.PrimeWeeks, prime - 1},
		{prime - 1, "prime", 0, 1, 1},
		{prime, "decline", 0, 0, 0},
		{prime + 1, "decline", 0, 0, 0},
		{prime + 604800000 - 1, "decline", 0, 0, 0},
		{prime + 604800000, "decline", 1, 0, 0},
		{end - 1, "decline", dna.DeclineWeeks - 1, 0, 0},
		{end, "retired", dna.DeclineWeeks, 0, 0},
		{end + 1, "retired", dna.DeclineWeeks, 0, 0},
		{end + 604800000*10000, "retired", dna.DeclineWeeks, 0, 0},
	} {
		got, err := Project(1, dna, born, born+test.offset, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got.Phase != test.phase || got.ElapsedDeclineWeeks != test.step || got.PrimeRemainingWeeks != test.remainWeeks || got.PrimeRemainingMS != test.remainMS {
			t.Fatal("boundary mismatch", test, got)
		}
		if test.offset >= end && got.CareerRemainingMS != 0 {
			t.Fatal("retirement duration continued")
		}
		if test.step == 0 && got.AgedAttributesTenths != (AttributesTenths{dna.Attributes.Power * 10, dna.Attributes.Speed * 10, dna.Attributes.Chin * 10, dna.Attributes.Stamina * 10, dna.Attributes.Skill * 10}) {
			t.Fatal("loss before first completed decline week")
		}
	}
	final, err := Project(1, dna, born, born+end, 0)
	if err != nil {
		t.Fatal(err)
	}
	later, err := Project(1, dna, born, math.MaxInt64, 0)
	if err != nil {
		t.Fatal(err)
	}
	if final.AgedAttributesTenths != later.AgedAttributesTenths || final.RatingHundredths != later.RatingHundredths {
		t.Fatal("continued decline after retirement")
	}
}

func TestExperienceIsAddedAfterAging(t *testing.T) {
	dna := testDNA(t)
	original := dna
	for week := 0; week <= dna.PrimeWeeks+dna.DeclineWeeks+1; week++ {
		at := int64(1) + int64(week)*604800000
		base, err := Project(1, dna, 1, at, 0)
		if err != nil {
			t.Fatal(err)
		}
		for xp := 1; xp <= 15; xp++ {
			got, err := Project(1, dna, 1, at, xp)
			if err != nil {
				t.Fatal(err)
			}
			if got.AgedAttributesTenths != base.AgedAttributesTenths {
				t.Fatal("XP entered aging")
			}
			league := got.LeagueAttributesTenths
			league.Skill -= 10 * xp
			if league != base.LeagueAttributesTenths || got.RatingHundredths-base.RatingHundredths != 80*xp {
				t.Fatal("XP changed other values or rating")
			}
		}
	}
	if dna != original {
		t.Fatal("input DNA modified")
	}
	// Direct private-helper boundary: canonical original one ends below 1.0.
	if ageAttribute(1, 40, 13, 13) != 6 {
		t.Fatal("added attribute floor")
	}
	if got := rating(AttributesTenths{1000, 1000, 1000, 1000, 1150}); got != 50200 {
		t.Fatal("rating added cap/rounding", got)
	}
}

func TestConcurrentPureProjection(t *testing.T) {
	dna := testDNA(t)
	before := dna
	want, err := Project(1, dna, 1, math.MaxInt64, 15)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 12; n++ {
				got, err := Project(1, dna, 1, math.MaxInt64, 15)
				if err != nil || got != want {
					t.Error("nondeterministic projection", err)
					return
				}
				got.LeagueAttributesTenths.Skill = 0
			}
		}()
	}
	wg.Wait()
	if dna != before {
		t.Fatal("DNA input changed")
	}
}

func FuzzProjectBounds(f *testing.F) {
	dna, err := fighter.Generate(1, [32]byte{})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(uint32(1), int64(1), int64(1), 0)
	f.Add(uint32(1), int64(1), int64(math.MaxInt64), 15)
	f.Add(uint32(1), int64(math.MaxInt64), int64(math.MaxInt64), 15)
	f.Add(uint32(0), int64(math.MinInt64), int64(math.MaxInt64), -1)
	for _, week := range []int{dna.PrimeWeeks, dna.PrimeWeeks + 1, dna.PrimeWeeks + dna.DeclineWeeks} {
		for _, delta := range []int64{-1, 0, 1} {
			f.Add(uint32(1), int64(1), 1+int64(week)*604800000+delta, 7)
		}
	}
	f.Fuzz(func(t *testing.T, version uint32, born, asof int64, xp int) {
		got, err := Project(version, dna, born, asof, xp)
		valid := version == 1 && born > 0 && asof >= born && xp >= 0 && xp <= 15
		if !valid {
			if err == nil || got != (Snapshot{}) {
				t.Fatal("invalid input or partial result")
			}
			return
		}
		if err != nil {
			t.Fatal("valid input rejected", err)
		}
		if got.AgeWeeks < 0 || got.ElapsedDeclineWeeks < 0 || got.ElapsedDeclineWeeks > dna.DeclineWeeks || got.PrimeRemainingWeeks < 0 || got.PrimeRemainingWeeks > dna.PrimeWeeks || got.PrimeRemainingMS < 0 || got.CareerRemainingMS < 0 || got.RatingHundredths < 0 || got.RatingHundredths > 50200 {
			t.Fatal("projection bounds", got)
		}
		if got.Seed != dna.Seed || got.BornAtMS != born || got.AsOfMS != asof {
			t.Fatal("fact mutation")
		}
	})
}
