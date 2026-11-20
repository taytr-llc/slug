// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterevidence

// This closed schema is deliberately explicit. A later DTO field cannot
// silently extend the frozen archive format (PUBLIC_FIGHTER_EVIDENCE.md).
type field struct {
	name string
	node *shape
}
type shape struct {
	kind      byte
	fields    []field
	elem      *shape
	min, max  int
	hex       bool
	maxNumber uint64
	maxBytes  int
}

func member(name string, node *shape) field { return field{name, node} }
func object(limit int, fields ...field) *shape {
	return &shape{kind: 'o', fields: fields, maxBytes: limit}
}
func array(min, max int, elem *shape) *shape {
	return &shape{kind: 'a', min: min, max: max, elem: elem}
}
func stringShape(min, max int, hex bool) *shape {
	return &shape{kind: 's', min: min, max: max, hex: hex}
}

var (
	textShape    = stringShape(0, 4096, false)
	hashShape    = stringShape(64, 64, true)
	keyShape     = stringShape(66, 66, true)
	u32Shape     = &shape{kind: 'n', maxNumber: 1<<32 - 1}
	u64Shape     = &shape{kind: 'n', maxNumber: 1<<64 - 1}
	i64Shape     = &shape{kind: 'n', maxNumber: 1<<63 - 1}
	safeShape    = &shape{kind: 'n', maxNumber: 1<<53 - 1}
	boolShape    = &shape{kind: 'b'}
	contextShape = object(8192,
		member("version", u32Shape), member("network", u32Shape), member("genesis", hashShape), member("dna_version", u32Shape),
		member("prospect_target", hashShape), member("parent_hash", hashShape), member("parent_height", u32Shape),
		member("expected_bits", u32Shape), member("min_timestamp", u32Shape), member("max_timestamp", u32Shape))
	dnaShape = object(8192,
		member("dna_version", u32Shape), member("seed", hashShape), member("rarity", textShape), member("talent_budget", i64Shape),
		member("attributes", object(1024, member("power", i64Shape), member("speed", i64Shape), member("chin", i64Shape), member("stamina", i64Shape), member("skill", i64Shape))),
		member("prime_weeks", i64Shape), member("decline_weeks", i64Shape))
	proofShape = object(65536, member("version", u32Shape), member("gym_id", hashShape), member("header", stringShape(160, 160, true)),
		member("coinbase", stringShape(2, 32768, true)), member("merkle_branch", array(0, 20, hashShape)), member("job_id", hashShape))
	jobShape = object(8192, member("version", u32Shape), member("rule_version", u32Shape), member("id", hashShape), member("authority_id", hashShape),
		member("descriptor_id", hashShape), member("context", contextShape), member("public_key", keyShape), member("gym_id", hashShape), member("challenge", hashShape),
		member("issued_at_ms", i64Shape), member("expires_at_ms", i64Shape))
	admissionShape = object(65536, member("version", u32Shape), member("sequence", u64Shape), member("context", contextShape), member("proof", proofShape),
		member("verified", object(16384, member("version", u32Shape), member("verification", textShape), member("network", u32Shape), member("genesis", hashShape),
			member("prospect_id", hashShape), member("policy_id", hashShape), member("gym_id", hashShape), member("work_hash", hashShape),
			member("parent_hash", hashShape), member("parent_height", u32Shape), member("dna", dnaShape))),
		member("authorization", object(512, member("version", u32Shape), member("public_key", keyShape), member("signature", stringShape(16, 144, true)))),
		member("job", jobShape), member("birth", object(512, member("rule_version", u32Shape), member("born_at_ms", i64Shape), member("accepted_at_ms", i64Shape))),
		member("eligibility", object(512, member("rule_version", u32Shape), member("source", textShape), member("hash", hashShape), member("height", u32Shape))),
		member("static_binding", object(512, member("version", u32Shape), member("descriptor_id", hashShape), member("manifest_digest", hashShape), member("portrait_digest", hashShape))))
	pinShape = object(4096, member("version", u32Shape), member("account_authority_id", hashShape), member("account_id", hashShape),
		member("identity", object(2048, member("network", u32Shape), member("genesis", hashShape), member("profile", textShape), member("root_public_key", textShape), member("gym_public_key", keyShape), member("gym_id", hashShape))),
		member("source", object(512, member("version", u32Shape), member("network", u32Shape), member("genesis", hashShape), member("authority_id", hashShape), member("descriptor_id", hashShape))),
		member("condition_version", u32Shape), member("projector_id", textShape))
	statusShape = object(4096, member("version", u32Shape), member("store_id", hashShape), member("pin", pinShape),
		member("checkpoint", safeShape), member("observed_source_count", safeShape), member("caught_up", boolShape), member("roster_revision", safeShape),
		member("slots", array(10, 10, textShape)), member("operation_count", u32Shape), member("pending_operation_id", textShape), member("last_operation_id", textShape), member("last_as_of_ms", safeShape))
	activationShape = object(512, member("version", u32Shape), member("policy_id", textShape), member("activation_id", hashShape), member("result_store_id", hashShape),
		member("result_created_at_ms", safeShape), member("source_created_at_ms", safeShape), member("activated_at_ms", safeShape))
	assignmentShape = object(2048, member("version", u32Shape), member("sequence", safeShape), member("prospect_id", hashShape), member("gym_id", hashShape), member("work_hash", hashShape),
		member("born_at_ms", safeShape), member("accepted_at_ms", safeShape), member("descriptor_id", hashShape), member("manifest_digest", hashShape), member("portrait_digest", hashShape), member("source_record_digest", hashShape))
	totalsShape  = object(128, member("wins", u32Shape), member("losses", u32Shape), member("draws", u32Shape), member("xp", u32Shape), member("base_points", u32Shape))
	captureShape = object(8192, member("version", u32Shape), member("status", statusShape), member("activation", activationShape), member("career_digest", hashShape),
		member("as_of_ms", safeShape), member("assignment", assignmentShape), member("prefix_count", u32Shape), member("event_count", u32Shape), member("totals", totalsShape), member("history_id", hashShape))
	cornerShape = object(512, member("prospect_id", hashShape), member("account_id", hashShape), member("gym_id", hashShape))
	eventShape  = object(1344, member("version", u32Shape), member("award_id", hashShape), member("result_key", hashShape), member("card_id", hashShape), member("bout_id", hashShape),
		member("source_record_digest", hashShape), member("input_id", hashShape), member("result_digest", hashShape), member("finality_digest", hashShape), member("effective_at_ms", safeShape),
		member("saturday", textShape), member("corner_a", cornerShape), member("corner_b", cornerShape), member("winner", textShape), member("method", textShape))
	rowShape     = object(1536, member("ordinal", u32Shape), member("event", eventShape), member("earned", totalsShape), member("xp_before", u32Shape), member("xp_applied", u32Shape), member("totals_after", totalsShape))
	archiveShape = object(MaxSize, member("format", textShape), member("version", u32Shape), member("admission", admissionShape), member("capture", captureShape), member("rows", array(0, MaxRows, rowShape)))
)
