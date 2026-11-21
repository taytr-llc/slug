// Copyright (c) 2026 The SLUG developers. ISC license.
package poolcapacity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"reflect"
	"testing"
)

type dailyReferenceCase struct {
	Name            string           `json:"name"`
	Input           DailyInputs      `json:"input"`
	Submissions     uint64           `json:"submissions"`
	Blocks          uint64           `json:"blocks"`
	Classification  string           `json:"classification"`
	Reservation     DailyReservation `json:"reservation"`
	ReservationJSON string           `json:"reservation_json"`
}
type dailyReferenceEnvelope struct {
	Name                 string           `json:"name"`
	Input                DailyInputs      `json:"input"`
	Classification       string           `json:"classification"`
	Maximum              uint64           `json:"maximum"`
	Reservation          DailyReservation `json:"reservation"`
	ReservationJSON      string           `json:"reservation_json"`
	ProbeCount           int              `json:"probe_count"`
	ExhaustivePredicates int              `json:"exhaustive_predicates"`
	Next                 struct {
		N              uint64           `json:"n"`
		Classification string           `json:"classification"`
		Reservation    DailyReservation `json:"reservation"`
	} `json:"next_failing_ordinary"`
}
type dailyReferenceCorpus struct {
	Version        uint32                   `json:"version"`
	SourceReady    string                   `json:"source_ready_sha256"`
	Recoveries     []dailyReferenceCase     `json:"recoveries"`
	Envelopes      []dailyReferenceEnvelope `json:"envelopes"`
	Invalid        []dailyReferenceCase     `json:"invalid_inputs"`
	ByteEdges      []dailyReferenceCase     `json:"byte_edges"`
	Malformed      []json.RawMessage        `json:"malformed_inputs"`
	FrameJSONBytes map[string]uint64        `json:"frame_json_bytes"`
	FormulaFrames  map[string]uint64        `json:"formula_frames"`
	Fixed          struct {
		Reservation DailyReservation `json:"reservation"`
	} `json:"fixed_grammar_maxima"`
	Fraction struct {
		LCM               string `json:"lcm"`
		CommonDenominator string `json:"common_denominator"`
		Exponent          uint   `json:"power_bound_exponent"`
		DenominatorDigits int    `json:"bound_denominator_digits"`
		NumeratorDigits   int    `json:"bound_numerator_digits"`
	} `json:"fraction_proof"`
	Overflow struct {
		Value         string `json:"max_money_times_rows"`
		ExceedsUint64 bool   `json:"exceeds_uint64"`
	} `json:"exact_overflow_control"`
}

func loadDailyCapacityReference(t *testing.T) dailyReferenceCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/daily-reservation/reference-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "2db086115f51b0c7e22d2fd7688ad5b46c56e238fa1346d709c9a19bebb7aa78" {
		t.Fatal("independent DAILY capacity corpus changed")
	}
	var corpus dailyReferenceCorpus
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Version != 1 || corpus.SourceReady != "625fc3cb19afe403bb4782051152e966dbbb077e13cc6d9ef38fb24534168e89" ||
		len(corpus.Recoveries) != 14 || len(corpus.Envelopes) != 6 || len(corpus.Invalid) != 14 ||
		len(corpus.ByteEdges) != 34 || len(corpus.Malformed) != 9 {
		t.Fatal("independent DAILY case inventory")
	}
	return corpus
}

func checkDailyReferenceResult(t *testing.T, classification string, expected DailyReservation,
	expectedJSON string, got DailyReservation, err error) {
	t.Helper()
	switch classification {
	case "accept":
		if err != nil || got != expected {
			t.Fatalf("reservation got%+v want%+v error%v", got, expected, err)
		}
	case "invalid", "capacity":
		want := ErrCapacity
		if classification == "invalid" {
			want = ErrInvalid
		}
		if err != want || got != (DailyReservation{}) || expected != (DailyReservation{}) {
			t.Fatalf("refusal classification%s result%+v error%v", classification, got, err)
		}
	default:
		t.Fatal("unknown independent classification", classification)
	}
	if expectedJSON != "" {
		raw, e := json.Marshal(got)
		if e != nil || string(raw) != expectedJSON {
			t.Fatal("canonical reservation bytes", string(raw), expectedJSON, e)
		}
	}
}

func TestDailyRecoveryIndependentReference(t *testing.T) {
	corpus := loadDailyCapacityReference(t)
	groups := []struct {
		name string
		rows []dailyReferenceCase
	}{
		{"recovery", corpus.Recoveries}, {"invalid", corpus.Invalid}, {"byte-edge", corpus.ByteEdges},
	}
	for _, group := range groups {
		t.Run(group.name, func(t *testing.T) {
			for _, row := range group.rows {
				t.Run(row.Name, func(t *testing.T) {
					got, err := DailyRecovery(row.Input, row.Submissions, row.Blocks)
					checkDailyReferenceResult(t, row.Classification, row.Reservation, row.ReservationJSON, got, err)
				})
			}
		})
	}
	// API inputs are uint64 values. Exercise extremes before any count products;
	// raw JSON grammar controls in the corpus belong to the Python producer only.
	base := corpus.Envelopes[0].Input
	for _, row := range []struct {
		n, r uint64
		err  error
	}{
		{^uint64(0), ^uint64(0), ErrCapacity}, {0, ^uint64(0), ErrInvalid},
		{^uint64(0), 0, ErrCapacity},
	} {
		got, err := DailyRecovery(base, row.n, row.r)
		if err != row.err || got != (DailyReservation{}) {
			t.Fatal("extreme count refusal", got, err)
		}
	}
	for _, row := range corpus.Invalid {
		n, got, err := DailyEnvelope(row.Input)
		if n != 0 || got != (DailyReservation{}) || err != ErrInvalid {
			t.Fatal("invalid envelope granted", row.Name, n, got, err)
		}
	}
}

func TestDailyEnvelopeIndependentReference(t *testing.T) {
	corpus := loadDailyCapacityReference(t)
	for _, row := range corpus.Envelopes {
		t.Run(row.Name, func(t *testing.T) {
			n, got, err := DailyEnvelope(row.Input)
			checkDailyReferenceResult(t, row.Classification, row.Reservation, row.ReservationJSON, got, err)
			if n != row.Maximum || row.ExhaustivePredicates != 4096 || row.ProbeCount < 1 || row.ProbeCount > 13 {
				t.Fatal("derived ordinary maximum/reference search inventory", n, row.Maximum, row.ProbeCount)
			}
			next, e := dailyOrdinary(row.Input, row.Next.N)
			checkDailyReferenceResult(t, row.Next.Classification, row.Next.Reservation, "", next, e)
			if err == nil {
				if got.Submissions != n+1 || got.Blocks != n+1 {
					t.Fatal("ceiling failed to reserve extra winner", got)
				}
				// The full ordinary history is not a fresh counter at recovery. Its
				// additional all-block candidate must be the exact reserved history.
				recovered, e := DailyRecovery(row.Input, n+1, n+1)
				if e != nil || recovered != got {
					t.Fatal("reserved whole-history recovery", recovered, got, e)
				}
			}
		})
	}
	if corpus.Envelopes[0].Maximum <= 721 || corpus.Envelopes[0].Maximum != 4095 ||
		corpus.Envelopes[0].ProbeCount != 13 || corpus.Envelopes[len(corpus.Envelopes)-1].ProbeCount != 1 {
		t.Fatal("useful capacity/all-fit/none-fit reference endpoints")
	}
}

func TestDailyCapacityProofFrames(t *testing.T) {
	corpus := loadDailyCapacityReference(t)
	frames := map[string]string{
		"credit": dailyCreditFrame, "original_claim": dailyOriginalClaimFrame,
		"input_reference": dailyInputReferenceFrame, "payment": dailyPaymentFrame,
		"change": dailyChangeFrame, "batch": dailyBatchFrame, "reference": dailyReferenceFrame,
		"allocation": dailyAllocationFrame, "round": dailyRoundFrame, "balance": dailyBalanceFrame,
		"report": dailyReportFrame, "residual_claim": dailyResidualClaimFrame,
	}
	recipe, err := os.ReadFile("testdata/daily-reservation/reference/inputs/frames.json")
	if err != nil {
		t.Fatal(err)
	}
	frameDigest := sha256.Sum256(recipe)
	if hex.EncodeToString(frameDigest[:]) != "0dc72b08c53112f1fd638463dc498106dc1beb727b0057c730127fa90601a361" {
		t.Fatal("independent literal frame recipe changed")
	}
	var source struct {
		Frames map[string]json.RawMessage `json:"frames"`
	}
	if err = json.Unmarshal(recipe, &source); err != nil || len(source.Frames) != len(frames) {
		t.Fatal("independent literal frame inventory", err)
	}
	lengths := make(map[string]uint64, len(frames))
	for name, raw := range frames {
		var compact bytes.Buffer
		if err = json.Compact(&compact, source.Frames[name]); err != nil || compact.String() != raw {
			t.Fatal("literal frame differs from frozen recipe", name, err)
		}
		lengths[name] = uint64(len(raw))
	}
	if !reflect.DeepEqual(lengths, corpus.FrameJSONBytes) {
		t.Fatal("literal frame bytes", lengths, corpus.FrameJSONBytes)
	}
	f := dailyFrames()
	formulas := map[string]uint64{"F_c": f.credit, "F_q": f.claim, "F_i": f.input, "F_p": f.payment,
		"F_b": f.batch, "F_x": f.reference, "F_a": f.allocation, "F_r": f.round,
		"F_balance": f.balance, "F_report": f.report, "F_active": f.active}
	if !reflect.DeepEqual(formulas, corpus.FormulaFrames) {
		t.Fatal("formula frames", formulas, corpus.FormulaFrames)
	}
	fixed, ok := dailyBounds(16384, 16384, 256)
	if !ok || fixed != corpus.Fixed.Reservation {
		t.Fatal("fixed grammar diagnostic", fixed, corpus.Fixed.Reservation)
	}
	// This maximal tuple proves codec widths only; its monetary history refuses.
	in := corpus.Envelopes[1].Input
	got, err := DailyRecovery(in, 16384, 16384)
	if err != ErrCapacity || got != (DailyReservation{}) {
		t.Fatal("grammar bound became monetary authority", got, err)
	}
	if fixed.ManagerCopyBytes > dailyManagerBytes || fixed.SigningBytes > dailySigningBytes ||
		fixed.LargestCreditBytes > dailyCreditBytes || fixed.LargestBatchBytes > dailyBodyBytes ||
		fixed.LargestRoundBytes > dailyRoundBytes || fixed.ReportBytes > dailyReportBytes ||
		fixed.LedgerBytes > dailyLogicalBytes {
		t.Fatal("independent maximum exceeds selected codec/request bound")
	}
	// Independent exact values are not cast to machine integers before checking.
	overflow := new(big.Int).Mul(new(big.Int).SetUint64(dailyMaxMoney), new(big.Int).SetUint64(dailyMaximumRows))
	if overflow.String() != corpus.Overflow.Value || overflow.IsUint64() || !corpus.Overflow.ExceedsUint64 {
		t.Fatal("overflow proof", overflow)
	}
	lcm, ok := new(big.Int).SetString(corpus.Fraction.LCM, 10)
	if !ok || lcm.Sign() <= 0 {
		t.Fatal("reference LCM")
	}
	denominator, ok := new(big.Int).SetString(corpus.Fraction.CommonDenominator, 10)
	if !ok || new(big.Int).Mul(big.NewInt(100), lcm).Cmp(denominator) != 0 {
		t.Fatal("reference denominator")
	}
	upperD := new(big.Int).Mul(big.NewInt(100), new(big.Int).Lsh(big.NewInt(1), corpus.Fraction.Exponent))
	upperN := new(big.Int).Mul(overflow, upperD)
	if denominator.Cmp(upperD) > 0 || len(upperD.String()) != corpus.Fraction.DenominatorDigits ||
		len(upperN.String()) != corpus.Fraction.NumeratorDigits ||
		uint64(len(upperD.String())) > dailyDenominatorDigits || uint64(len(upperN.String())) > dailyNumeratorDigits {
		t.Fatal("common-denominator fraction width")
	}
}
