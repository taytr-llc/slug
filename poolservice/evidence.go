// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectwork"
)

// SubmissionMatches checks receipt structure against independently checked
// work. It does not hash, authenticate a service or establish credit by itself.
// Journal readers may use a claimed digest for structural auditing, but must
// verify PoW before acting on retained evidence.
func SubmissionMatches(source poolshare.Config, work poolshare.MiningWork, nonce uint32, check prospectwork.ShareCheck, receipt poolshare.Submission) bool {
	return source.Version == poolshare.Version1 && validSubmission(source, work, nonce, check, receipt)
}
