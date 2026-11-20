// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import "github.com/taytr-llc/slug/gymevent"

const (
	MaxCardSourceSize = 160 * 1024
	MaxCardStateSize  = 12 * 1024
	MaxCardViewSize   = 120 * 1024
)

type CardSource struct {
	Version       uint32                   `json:"version"`
	StoreID       string                   `json:"store_id"`
	EventKey      string                   `json:"event_key"`
	SourceReceipt gymevent.SnapshotReceipt `json:"source_receipt"`
	Declaration   SignedDeclaration        `json:"declaration"`
	Anchor        Anchor                   `json:"anchor"`
	Target        Target                   `json:"target"`
	Finality      Finality                 `json:"finality"`
}
type CardState struct {
	Version      uint32 `json:"version"`
	Status       Status `json:"status"`
	SourceDigest string `json:"source_digest"`
}
type CardView struct {
	Version              uint32      `json:"version"`
	StoreID              string      `json:"store_id"`
	Pin                  Pin         `json:"pin"`
	CreatedAtMS          int64       `json:"created_at_ms"`
	ParticipantAccountID string      `json:"participant_account_id"`
	ParticipantGymID     string      `json:"participant_gym_id"`
	ObservedAtMS         int64       `json:"observed_at_ms"`
	States               []CardState `json:"states"`
}
