// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercareer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestIndependentArithmeticReference(t *testing.T) {
	raw, err := os.ReadFile("testdata/reference/corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	// These bytes were independently generated from prose before this Go
	// implementation existed. Never replace the corpus with runtime output.
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "63e85d8b3a6aeadddcf34c4a71cc70fc87e24f9335be37786edddfe8843c6b9d" {
		t.Fatal("independent reference changed")
	}
	var corpus struct {
		Version     uint32            `json:"version"`
		InputSHA256 map[string]string `json:"input_sha256"`
		Awards      []struct {
			ID    string `json:"id"`
			Input struct {
				Winner, Method, Kind string
			} `json:"input"`
			Expected struct {
				Error  string   `json:"error"`
				Deltas [2]Delta `json:"deltas"`
			} `json:"expected"`
		} `json:"award_cases"`
		Applies []struct {
			ID    string `json:"id"`
			Input struct {
				Prior Totals `json:"prior"`
				Delta Delta  `json:"delta"`
			} `json:"input"`
			Expected struct {
				Error  string `json:"error"`
				Totals Totals `json:"totals"`
			} `json:"expected"`
		} `json:"apply_cases"`
		Quality []struct {
			ID    string `json:"id"`
			Input struct {
				Rank uint32 `json:"opponent_rank"`
			} `json:"input"`
			Expected struct {
				Error string `json:"error"`
				Bonus uint32 `json:"bonus"`
			} `json:"expected"`
		} `json:"quality_cases"`
		Sequences []struct {
			ID      string `json:"id"`
			Initial Totals `json:"initial"`
			Steps   []struct {
				Delta  Delta  `json:"delta"`
				Totals Totals `json:"totals"`
			} `json:"steps"`
		} `json:"apply_sequences"`
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Version != 1 || len(corpus.Awards) != 63 || len(corpus.Applies) != 98 || len(corpus.Quality) != 10 || len(corpus.Sequences) != 3 || len(corpus.InputSHA256) != 2 {
		t.Fatal("incomplete independent corpus")
	}
	for path, want := range corpus.InputSHA256 {
		data, err := os.ReadFile("testdata/reference/" + path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			t.Fatalf("reference input changed: %s", path)
		}
	}
	checkError := func(t *testing.T, err error, want string) {
		t.Helper()
		switch want {
		case "":
			if err != nil {
				t.Fatal(err)
			}
		case "ErrInvalid":
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected ErrInvalid, got %v", err)
			}
		case "ErrOverflow":
			if !errors.Is(err, ErrOverflow) {
				t.Fatalf("expected ErrOverflow, got %v", err)
			}
		default:
			t.Fatal("unknown reference error", want)
		}
	}
	for _, c := range corpus.Awards {
		t.Run("award/"+c.ID, func(t *testing.T) {
			got, err := Award(c.Input.Winner, c.Input.Method, c.Input.Kind)
			checkError(t, err, c.Expected.Error)
			if err == nil && got != c.Expected.Deltas {
				t.Fatalf("got %+v, want %+v", got, c.Expected.Deltas)
			}
		})
	}
	for _, c := range corpus.Applies {
		t.Run("apply/"+c.ID, func(t *testing.T) {
			got, err := Apply(c.Input.Prior, c.Input.Delta)
			checkError(t, err, c.Expected.Error)
			if err == nil && got != c.Expected.Totals {
				t.Fatalf("got %+v, want %+v", got, c.Expected.Totals)
			}
		})
	}
	for _, c := range corpus.Quality {
		t.Run("quality/"+c.ID, func(t *testing.T) {
			got, err := QualityBonus(c.Input.Rank)
			checkError(t, err, c.Expected.Error)
			if err == nil && got != c.Expected.Bonus {
				t.Fatalf("got %d, want %d", got, c.Expected.Bonus)
			}
		})
	}
	for _, c := range corpus.Sequences {
		t.Run("sequence/"+c.ID, func(t *testing.T) {
			prior := c.Initial
			for _, step := range c.Steps {
				got, err := Apply(prior, step.Delta)
				if err != nil || got != step.Totals {
					t.Fatalf("got %+v, %v; want %+v", got, err, step.Totals)
				}
				prior = got
			}
		})
	}
}
