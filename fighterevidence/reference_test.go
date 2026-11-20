// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterevidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/wire/v2"
)

type referencePositive struct {
	Name      string        `json:"name"`
	Pin       gymstable.Pin `json:"expected_pin"`
	Canonical string        `json:"canonical"`
	Report    Report        `json:"report"`
}
type referenceNegative struct {
	Name      string        `json:"name"`
	Pin       gymstable.Pin `json:"expected_pin"`
	Raw       string        `json:"raw"`
	WorkCalls int           `json:"work_calls"`
	Refusal   string        `json:"refusal_category"`
}
type referenceCorpus struct {
	Positives []referencePositive `json:"positives"`
	Negatives []referenceNegative `json:"negatives"`
}

func reference(t *testing.T) referenceCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/reference/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if checksum(raw) != "eb51028daf2ebbda44dfbfdb79f58353137ed03f1836fad5b8db8cf428300772" {
		t.Fatal("frozen independent corpus changed")
	}
	var c referenceCorpus
	if json.Unmarshal(raw, &c) != nil || len(c.Positives) != 28 || len(c.Negatives) != 44 {
		t.Fatal("invalid independent corpus")
	}
	return c
}
func positive(t *testing.T, name string) referencePositive {
	t.Helper()
	for _, p := range reference(t).Positives {
		if p.Name == name {
			return p
		}
	}
	t.Fatal("missing case", name)
	return referencePositive{}
}

func TestEvidenceIndependentCheapBytes(t *testing.T) {
	for _, p := range reference(t).Positives {
		t.Run(p.Name, func(t *testing.T) {
			a, err := Parse([]byte(p.Canonical))
			if err != nil {
				t.Fatal(err)
			}
			checked, err := ValidateSuppliedShape(p.Pin, a)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := Marshal(p.Pin, checked)
			if err != nil || string(raw) != p.Canonical {
				t.Fatal("independent canonical mismatch", err)
			}
			if checksum(raw) != p.Report.FileSHA256 || len(raw) != p.Report.FileBytes {
				t.Fatal("independent file integrity mismatch")
			}
			if len(checked.Rows) > 0 {
				checked.Rows[0].Event.Method = "changed"
				if a.Rows[0].Event.Method == "changed" {
					t.Fatal("owned rows alias")
				}
			}
			if len(checked.Admission.Proof.MerkleBranch) > 0 {
				checked.Admission.Proof.MerkleBranch[0] = "changed"
				if a.Admission.Proof.MerkleBranch[0] == "changed" {
					t.Fatal("owned proof alias")
				}
			}
		})
	}
}

func TestEvidenceIndependentNormalWork(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal work qualification")
	}
	for _, p := range reference(t).Positives {
		if !strings.HasPrefix(p.Name, "actual/") {
			continue
		}
		t.Run(p.Name, func(t *testing.T) {
			calls := 0
			got, err := verifySupplied(t.Context(), p.Pin, []byte(p.Canonical), func(c prospect.Context, p prospect.Proof) (prospect.Verified, error) {
				calls++
				return prospect.Verify(c, p)
			})
			if err != nil || calls != 1 || got != p.Report {
				t.Fatal("independent normal-work report mismatch", calls, err, got, p.Report)
			}
		})
	}
}

func TestEvidenceIndependentRefusals(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal verification entry point")
	}
	for _, n := range reference(t).Negatives {
		t.Run(n.Name, func(t *testing.T) {
			calls := 0
			got, err := verifySupplied(t.Context(), n.Pin, []byte(n.Raw), func(c prospect.Context, p prospect.Proof) (prospect.Verified, error) {
				calls++
				return prospect.Verify(c, p)
			})
			want := ErrInvalid
			if n.Refusal == "source_mismatch" {
				want = ErrSourceMismatch
			}
			if !errors.Is(err, want) || got != (Report{}) || calls != n.WorkCalls {
				t.Fatal("independent refusal/work-count mismatch", err, want, calls, n.WorkCalls)
			}
		})
	}
}

func TestEvidenceParserInheritedIntegerWidths(t *testing.T) {
	p := positive(t, "actual/v1/zero256-unawarded")
	a, err := Parse([]byte(p.Canonical))
	if err != nil {
		t.Fatal(err)
	}
	// Frozen parser-only controls independently confirmed in the reference's
	// parser-boundaries.json; these extreme values are never valid DNA.
	fields := map[string]int{"talent_budget": a.Admission.Verified.DNA.TalentBudget, "power": a.Admission.Verified.DNA.Attributes.Power,
		"speed": a.Admission.Verified.DNA.Attributes.Speed, "chin": a.Admission.Verified.DNA.Attributes.Chin, "stamina": a.Admission.Verified.DNA.Attributes.Stamina,
		"skill": a.Admission.Verified.DNA.Attributes.Skill, "prime_weeks": a.Admission.Verified.DNA.PrimeWeeks, "decline_weeks": a.Admission.Verified.DNA.DeclineWeeks}
	for key, old := range fields {
		for _, value := range []string{"4294967296", "9223372036854775807", "9223372036854775808"} {
			t.Run(key+"/"+value, func(t *testing.T) {
				raw := bytes.Replace([]byte(p.Canonical), []byte("\""+key+"\":"+strconv.Itoa(old)), []byte("\""+key+"\":"+value), 1)
				v, e := Parse(raw)
				if (e == nil) != (value != "9223372036854775808") {
					t.Fatal("inherited signed integer grammar", e)
				}
				if e == nil {
					if _, e = ValidateSuppliedShape(p.Pin, v); !errors.Is(e, ErrInvalid) {
						t.Fatal("invalid DNA became evidence", e)
					}
				}
			})
		}
	}
}

func TestEvidenceTypedBoundsAndSource(t *testing.T) {
	p := positive(t, "actual/v2/zero256-unawarded")
	for _, edit := range []func(*Archive){
		func(a *Archive) { a.Rows = nil }, func(a *Archive) { a.Admission.Proof.MerkleBranch = nil },
		func(a *Archive) { a.Admission.Proof.Coinbase = strings.Repeat("0", 32770) },
		func(a *Archive) { a.Rows = make([]gymstable.CareerHistoryRow, 257) },
		func(a *Archive) { a.Admission.Proof.Header = strings.ToUpper(a.Admission.Proof.Header) },
		func(a *Archive) {
			a.Admission.Authorization.Signature = strings.ToUpper(a.Admission.Authorization.Signature)
		},
	} {
		a, e := Parse([]byte(p.Canonical))
		if e != nil {
			t.Fatal(e)
		}
		edit(&a)
		if raw, e := Marshal(p.Pin, a); !errors.Is(e, ErrInvalid) || raw != nil {
			t.Fatal("typed bound/normalization accepted", e)
		}
	}
	a, _ := Parse([]byte(p.Canonical))
	pin := p.Pin
	pin.Source.AuthorityID = strings.Repeat("b", 64)
	if _, e := ValidateSuppliedShape(pin, a); !errors.Is(e, ErrSourceMismatch) {
		t.Fatal("foreign pin accepted", e)
	}
}

func TestEvidenceWorkGateCancellationAndInputOwnership(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal verification entry point")
	}
	p := positive(t, "actual/v2/zero256-unawarded")
	if r, e := VerifySupplied(nil, p.Pin, nil); !errors.Is(e, ErrInvalid) || r != (Report{}) {
		t.Fatal("nil context", e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if r, e := VerifySupplied(ctx, p.Pin, nil); !errors.Is(e, ErrCancelled) || r != (Report{}) {
		t.Fatal("pre-cancel", e)
	}
	for _, cancelWork := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelWork), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			type result struct {
				r Report
				e error
			}
			done := make(chan result, 1)
			raw := []byte(p.Canonical)
			go func() {
				r, e := verifySupplied(ctx, p.Pin, raw, func(c prospect.Context, proof prospect.Proof) (prospect.Verified, error) {
					close(entered)
					<-release
					a, _ := Parse([]byte(p.Canonical))
					return a.Admission.Verified, nil
				})
				done <- result{r, e}
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				close(release)
				<-done
				t.Fatal("work never entered")
			}
			if r, e := VerifySupplied(t.Context(), p.Pin, nil); !errors.Is(e, ErrBusy) || r != (Report{}) {
				close(release)
				<-done
				t.Fatal("concurrent call queued", e)
			}
			// No expectation is derived from the seam: success matches the frozen
			// independent report, while real normal work is qualified separately.
			raw[0] = 'x'
			if cancelWork {
				cancel()
				select {
				case <-done:
					close(release)
					t.Fatal("cancel detached entered work")
				case <-time.After(20 * time.Millisecond):
				}
			}
			close(release)
			got := <-done
			if cancelWork {
				if !errors.Is(got.e, ErrCancelled) || got.r != (Report{}) {
					t.Fatal("cancel published", got.e)
				}
			} else if got.e != nil || got.r != p.Report {
				t.Fatal("input was not owned", got.e)
			}
			if r, e := VerifySupplied(t.Context(), p.Pin, nil); !errors.Is(e, ErrInvalid) || r != (Report{}) {
				t.Fatal("gate was retained", e)
			}
		})
	}
}

func TestEvidenceCheapProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		if r, e := VerifySupplied(ctx, gymstable.Pin{}, nil); !errors.Is(e, ErrProfile) || r != (Report{}) {
			t.Fatal("profile must precede input", e)
		}
	}
}
