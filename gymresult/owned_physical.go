package gymresult

import (
	"github.com/taytr-llc/slug/chaincfg/v2"
)

func ownedValidateOutpoint(p OwnedPin, v Outpoint) error {
	return validateOutpoint(physicalOwnedPin(p), v)
}
func ownedValidateFundingCoin(p OwnedPin, v FundingCoin) error {
	return validateFundingCoin(physicalOwnedPin(p), v)
}
func ownedValidatePublication(p OwnedPin, v Publication) error {
	return validatePublication(physicalOwnedPin(p), v)
}
func ownedValidateCursor(p OwnedPin, v Cursor) error { return validateCursor(physicalOwnedPin(p), v) }
func ownedValidateTip(p OwnedPin, v Tip) error       { return validateTip(physicalOwnedPin(p), v) }
func ownedValidateScanRestart(p OwnedPin, v ScanRestart) error {
	return validateScanRestart(physicalOwnedPin(p), v)
}
func ownedValidateAnchor(p OwnedPin, v Anchor) error { return validateAnchor(physicalOwnedPin(p), v) }
func ownedValidateInclusion(p OwnedPin, v Inclusion) error {
	return validateInclusion(physicalOwnedPin(p), v)
}
func ownedValidatePriorInclusion(p OwnedPin, v PriorInclusion) error {
	return validatePriorInclusion(physicalOwnedPin(p), v)
}
func ownedValidateFinality(p OwnedPin, v Finality) error {
	return validateFinality(physicalOwnedPin(p), v)
}
func ownedValidateObservation(p OwnedPin, v Observation) error {
	return validateObservation(physicalOwnedPin(p), v)
}
func ownedValidCheckpoint(p OwnedPin, v chaincfg.SignedCheckpoint) bool {
	return validCheckpoint(physicalOwnedPin(p), v)
}
