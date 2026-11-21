// Copyright (c) 2026 The SLUG developers. ISC license.
package poolcapacity

import "math/big"

// DailyInputs binds immutable, independently selected DAILY source/round/ledger
// limits. This pure value grants no source, session, funding or signing authority.
type DailyInputs struct {
	SourceMax    uint64 `json:"source_max"`
	SourceBytes  uint64 `json:"source_bytes"`
	Rounds       uint64 `json:"rounds"`
	RoundBytes   uint64 `json:"round_bytes"`
	CreditRounds uint64 `json:"credit_rounds"`
	Batches      uint64 `json:"batches"`
	Gyms         uint64 `json:"gyms"`
}

// DailyReservation bounds all retained liabilities; no paid or pending batch is
// subtracted. It is not a payout promise, persisted grant or disk-space reserve.
type DailyReservation struct {
	Submissions        uint64 `json:"submissions"`
	Blocks             uint64 `json:"blocks"`
	MaxBatches         uint64 `json:"max_batches"`
	ClaimLeaves        uint64 `json:"claim_leaves"`
	LedgerBytes        uint64 `json:"ledger_bytes"`
	RoundBytes         uint64 `json:"round_bytes"`
	LargestRoundBytes  uint64 `json:"largest_round_bytes"`
	LargestCreditBytes uint64 `json:"largest_credit_bytes"`
	LargestBatchBytes  uint64 `json:"largest_batch_bytes"`
	ReportBytes        uint64 `json:"report_bytes"`
	ReceiptBytes       uint64 `json:"receipt_bytes"`
	SigningBytes       uint64 `json:"signing_bytes"`
	ManagerCopyBytes   uint64 `json:"manager_copy_bytes"`
}

const (
	dailyMaximumRows       uint64 = 16384
	dailyMaximumBatches    uint64 = 32767
	dailyWindow            uint64 = 4096
	dailyLogicalBytes      uint64 = 8 << 30
	dailyReportBytes       uint64 = 8 << 20
	dailyHeadBytes         uint64 = 128 << 10
	dailyCreditBytes       uint64 = 128 << 10
	dailyBodyBytes         uint64 = 2 << 20
	dailyRoundBytes        uint64 = 2 << 20
	dailySigningBytes      uint64 = 640 << 20
	dailyManagerBytes      uint64 = 320 << 20
	dailyMaximumClaims     uint64 = 32768
	dailyMaxMoney          uint64 = 2100000000000000
	dailyMaxInt64          uint64 = 1<<63 - 1
	dailyWorstWorkBytes    uint64 = (16 << 20) + (16 << 10)
	dailySubmissionBytes   uint64 = 4096
	dailyNumeratorDigits   uint64 = 2490
	dailyDenominatorDigits uint64 = 2468
)

// Literal upper-bound frames follow the separately frozen DAILY recipe, not
// financial encoders. Empty arrays/null values remain while variable bytes and
// an extra comma per element are also added. State/reason widths each bound all
// validated literals independently. The corpus pins every frame's exact bytes.
const (
	dailyCreditFrame         = `{"round_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","round_digest":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","coin":{"txid":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","index":4294967295,"atoms":2100000000000000,"script":"0014ffffffffffffffffffffffffffffffffffffffff","coinbase":false,"block_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","block_height":2147483647,"claims":[]}}`
	dailyOriginalClaimFrame  = `{"round_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","round_height":2147483647,"gym_id":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","amount":{"numerator":"860160000000000000000","denominator":"409600"}}`
	dailyInputReferenceFrame = `{"txid":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","index":4294967295}`
	dailyPaymentFrame        = `{"gym_id":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","script":"0014ffffffffffffffffffffffffffffffffffffffff","atoms":2100000000000000}`
	dailyChangeFrame         = `{"index":256,"atoms":2100000000000000,"script":"0014ffffffffffffffffffffffffffffffffffffffff","claim_count":32768,"claims_digest":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}`
	dailyBatchFrame          = `{"version":2,"sequence":32767,"day":"9999-12-31","created_at_ms":253402300799999,"inputs":[],"payments":[],"fee_atoms":2100000000000000,"change":{"index":256,"atoms":2100000000000000,"script":"0014ffffffffffffffffffffffffffffffffffffffff","claim_count":32768,"claims_digest":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},"txid":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","raw":""}`
	dailyReferenceFrame      = `{"kind":"credit","sequence":32767}`
	dailyAllocationFrame     = `{"gym_id":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","shares":4096,"numerator":"8601600000000000000","denominator":"4096"}`
	dailyRoundFrame          = `{"version":1,"sequence":16384,"block_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","block_height":2147483647,"first_accepted":16384,"last_accepted":16384,"share_keys":[],"pool_target":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","payout_address":"pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp","reward_atoms":2100000000000000,"allocations":[],"winner":null}`
	dailyBalanceFrame        = `{"gym_id":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","confirmed":{"numerator":"","denominator":""},"available":{"numerator":"","denominator":""},"reserved_atoms":9223372036854775807,"paid_atoms":9223372036854775807,"reason":"rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr"}`
	dailyReportFrame         = `{"day":"9999-12-31","observed_at_ms":253402300799999,"txid":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","state":"ssssssssssssssssssssssssssssssss","reason":"rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr","balances":[]}`
	dailyResidualClaimFrame  = `{"round_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","round_height":2147483647,"gym_id":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","amount":{"numerator":"","denominator":""}}`
)

type dailyFrameSizes struct {
	credit, claim, input, payment, batch, reference uint64
	allocation, round, balance, report, active      uint64
}

func dailyFrames() dailyFrameSizes {
	return dailyFrameSizes{
		credit:     uint64(len(dailyCreditFrame)),
		claim:      uint64(len(dailyOriginalClaimFrame)) + 1,
		input:      uint64(len(dailyInputReferenceFrame)) + 1,
		payment:    uint64(len(dailyPaymentFrame)) + 1,
		batch:      uint64(len(dailyBatchFrame)),
		reference:  uint64(len(dailyReferenceFrame)),
		allocation: uint64(len(dailyAllocationFrame)) + 1,
		round:      uint64(len(dailyRoundFrame)),
		balance:    uint64(len(dailyBalanceFrame)) + 2*(dailyNumeratorDigits+dailyDenominatorDigits) + 1,
		report:     uint64(len(dailyReportFrame)),
		active:     uint64(len(dailyResidualClaimFrame)) + dailyNumeratorDigits + dailyDenominatorDigits + 1,
	}
}

func validDaily(in DailyInputs) bool {
	return in.SourceMax > 0 && in.SourceMax <= dailyMaximumRows &&
		in.SourceBytes >= 4096 && in.SourceBytes <= dailyLogicalBytes &&
		in.Rounds > 0 && in.Rounds <= dailyMaximumRows &&
		in.RoundBytes >= 4096 && in.RoundBytes <= dailyLogicalBytes &&
		in.CreditRounds > 0 && in.CreditRounds <= dailyMaximumRows &&
		in.Batches > 0 && in.Batches <= dailyMaximumBatches &&
		in.Gyms > 0 && in.Gyms <= 256
}

// DailyRecovery evaluates a complete original history plus any proposed rows.
// Blocks includes pending, orphaned and aliased block rows. The caller must
// authenticate the original complete N/R/gym/window census and actual work
// bytes; this pure calculation neither reads nor changes any journal.
func DailyRecovery(in DailyInputs, submissions, blocks uint64) (DailyReservation, error) {
	if !validDaily(in) || blocks > submissions {
		return DailyReservation{}, ErrInvalid
	}
	if submissions > in.SourceMax || blocks > in.Rounds || blocks > in.CreditRounds {
		return DailyReservation{}, ErrCapacity
	}
	// The selected limits establish n,r<=16384 before additions. Compare money
	// by division, never uint64(MaxMoney)*N (which overflows at daily maxima).
	if blocks > dailyMaxInt64/dailyMaxMoney || submissions+blocks > dailyMaximumClaims {
		return DailyReservation{}, ErrCapacity
	}
	out, ok := dailyBounds(submissions, blocks, in.Gyms)
	if !ok || out.MaxBatches > in.Batches || out.LedgerBytes > dailyLogicalBytes ||
		out.RoundBytes > in.RoundBytes || out.LargestRoundBytes > dailyRoundBytes ||
		out.LargestCreditBytes > dailyCreditBytes || out.LargestBatchBytes > dailyBodyBytes ||
		out.ReportBytes > dailyReportBytes || out.ReceiptBytes > in.SourceBytes ||
		out.SigningBytes > dailySigningBytes || out.ManagerCopyBytes > dailyManagerBytes {
		return DailyReservation{}, ErrCapacity
	}
	return out, nil
}

// DailyEnvelope reserves one additional canonical winner at the worst all-block
// history (n+1,n+1), plus work/receipt source bytes. The old 4096-share window
// leaves at most4095 ordinary rows; later recovery is conditional on whole N/R.
func DailyEnvelope(in DailyInputs) (maximum uint64, reserved DailyReservation, err error) {
	if !validDaily(in) {
		return 0, DailyReservation{}, ErrInvalid
	}
	best, err := dailyOrdinary(in, 0)
	if err != nil {
		return 0, DailyReservation{}, err
	}
	lo, hi := uint64(0), dailyWindow-1
	// Initial n=0 plus at most12 binary probes over the remaining4095 choices.
	// Every term is nondecreasing; there is no nested N/R or LCM scan.
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		candidate, e := dailyOrdinary(in, mid)
		if e == nil {
			lo, best = mid, candidate
		} else if e == ErrCapacity {
			hi = mid - 1
		} else {
			return 0, DailyReservation{}, e
		}
	}
	return lo, best, nil
}

func dailyOrdinary(in DailyInputs, n uint64) (DailyReservation, error) {
	if n >= dailyWindow {
		return DailyReservation{}, ErrCapacity
	}
	out, err := DailyRecovery(in, n+1, n+1)
	if err != nil {
		return DailyReservation{}, err
	}
	// n<4096, so this addition/multiplication has a small proved bound.
	if dailyWorstWorkBytes+(n+1)*dailySubmissionBytes > in.SourceBytes {
		return DailyReservation{}, ErrCapacity
	}
	return out, nil
}

// Each helper allocates a fresh integer, so no intermediate aliases a term that
// is reused in a later equation. All byte products are exact before conversion.
func dailyProduct(values ...uint64) *big.Int {
	out := big.NewInt(1)
	for _, value := range values {
		out.Mul(out, new(big.Int).SetUint64(value))
	}
	return out
}
func dailySum(values ...*big.Int) *big.Int {
	out := new(big.Int)
	for _, value := range values {
		out.Add(out, value)
	}
	return out
}

func dailyBounds(n, r, g uint64) (DailyReservation, bool) {
	if n > dailyMaximumRows || r > n || g == 0 || g > 256 {
		return DailyReservation{}, false
	}
	f := dailyFrames()
	var b, c uint64
	if r > 0 {
		b, c = 2*r-1, n+r
	}
	// These small counts are bounded before arithmetic; products below use big.Int.
	i := r + b
	p := dailyProduct(g, b)
	raw := dailySum(dailyProduct(16, b), dailyProduct(150, i),
		new(big.Int).Mul(big.NewInt(31), dailySum(p, dailyProduct(b))))
	credits := dailySum(dailyProduct(r, 10+f.credit), dailyProduct(c, f.claim))
	batches := dailySum(dailyProduct(b, 10+f.batch), dailyProduct(i, f.input),
		new(big.Int).Mul(p, new(big.Int).SetUint64(f.payment)), new(big.Int).Mul(big.NewInt(2), raw))
	indexes := dailySum(dailyProduct(r, 67+8), dailyProduct(b, 67+8), dailyProduct(b, 13+8),
		dailyProduct(r+b, 10+f.reference), dailyProduct(r+b, 70+f.reference), dailyProduct(i, 74+8))
	ledger := dailySum(dailyProduct(dailyHeadBytes+dailyReportBytes), credits, batches, indexes, dailyProduct(b, 11+256))
	rounds, largestRound, largestCredit, largestBody := new(big.Int), new(big.Int), new(big.Int), new(big.Int)
	if r > 0 {
		w := min(n, dailyWindow)
		rounds = dailySum(dailyProduct(r, f.round+65536+4096), dailyProduct(n, 67), dailyProduct(n, f.allocation))
		largestRound = dailySum(dailyProduct(f.round+65536), dailyProduct(w, 67), dailyProduct(min(g, w), f.allocation))
		largestCredit = dailySum(dailyProduct(f.credit), dailyProduct(min(g, dailyWindow)+1, f.claim))
		largestBody = dailySum(dailyProduct(f.batch), dailyProduct(1464, f.input), dailyProduct(256, f.payment), dailyProduct(2, 400000))
	}
	out := DailyReservation{Submissions: n, Blocks: r, MaxBatches: b, ClaimLeaves: c}
	fields := []struct {
		value  *big.Int
		target *uint64
	}{
		{ledger, &out.LedgerBytes}, {rounds, &out.RoundBytes},
		{largestRound, &out.LargestRoundBytes}, {largestCredit, &out.LargestCreditBytes},
		{largestBody, &out.LargestBatchBytes},
		{dailySum(dailyProduct(f.report), dailyProduct(g, f.balance)), &out.ReportBytes},
		{dailyProduct(n, dailySubmissionBytes), &out.ReceiptBytes},
		{dailySum(dailyProduct(2, c, f.active), dailyProduct(dailyReportBytes)), &out.SigningBytes},
		{dailySum(dailyProduct(c, 64+64+dailyNumeratorDigits+dailyDenominatorDigits+256), dailyProduct(1464, 512), dailyProduct(400000)), &out.ManagerCopyBytes},
	}
	for _, field := range fields {
		if !field.value.IsUint64() {
			return DailyReservation{}, false
		}
		*field.target = field.value.Uint64()
	}
	return out, true
}
