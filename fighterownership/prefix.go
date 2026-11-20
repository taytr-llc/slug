// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import "github.com/taytr-llc/slug/registry"

func verifyPublicationPrefix(pin Pin, values []PublicationEntry) error {
	if len(values) > 16384 {
		return ErrCapacity
	}
	previous := zeroID
	clock := int64(0)
	for n, v := range values {
		if v.Sequence != uint32(n+1) || v.PreviousID != previous || v.AttestedAtMS < clock {
			return ErrCorrupt
		}
		id, e := registry.PublicPublicationID(pin, v)
		if e != nil {
			return e
		}
		previous = id
		clock = v.AttestedAtMS
	}
	return nil
}
func verifyFramePrefix(pin Pin, values []FinalityFrame) error {
	if len(values) > 4096 {
		return ErrCapacity
	}
	previous := zeroID
	for n, v := range values {
		if v.Sequence != uint32(n+1) || v.PreviousBodyID != previous {
			return ErrCorrupt
		}
		if e := ValidateFinalityFrame(pin, v); e != nil {
			return e
		}
		id, e := objectDigest("SLUG fighter ownership finality body", v.FinalityFrameBody)
		if e != nil {
			return e
		}
		previous = id
	}
	return nil
}
