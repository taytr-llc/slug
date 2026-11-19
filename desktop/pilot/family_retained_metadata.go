// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
)

// These private charges cover the helper's retained authority, not the public
// metadata response maxima. Each includes the entire validated recursive Pin,
// 2KiB for the closed snapshot fields, and 4KiB for fixed private bookkeeping.
// The snapshot bound conservatively includes a full View, although the helper
// shares that immutable View with Stable. IDs, creation times, failure strings,
// flags and Result's node-owner references cannot grow an unbounded cache.
//
// Status/Record/History/CardStatus responses are transient here; the helper does
// not retain the native client's selected overlays or portraits. A terminal Job
// is charged separately in full. Public metadata/job caps and full Begin
// reservations stay unchanged, as does the aggregate 1MiB limit. These are
// serialized presentation-accounting bounds, not claims about process heap use.
const (
	cardRetainedMetadataBytes   = gymcard.MaxPinSize + 6*1024
	boutRetainedMetadataBytes   = gymbout.MaxPinSize + 6*1024
	resultRetainedMetadataBytes = gymresult.MaxPinSize + 6*1024
)
