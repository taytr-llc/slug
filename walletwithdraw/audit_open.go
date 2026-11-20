// Copyright (c) 2026 The SLUG developers. ISC license.
package walletwithdraw

import "context"

// AuditOwner retains an established, fully audited journal without exposing
// mutation/delivery methods. Unpublished stages remain untouched.
type AuditOwner struct{ journal *Journal }

func OpenAudit(ctx context.Context, path string, expected Identity) (*AuditOwner, error) {
	j, e := openJournalMode(ctx, path, expected, false, true)
	if e != nil {
		return nil, e
	}
	return &AuditOwner{j}, nil
}
func (a *AuditOwner) Close() error {
	if a == nil {
		return nil
	}
	return a.journal.Close()
}
