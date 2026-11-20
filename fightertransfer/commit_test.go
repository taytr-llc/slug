// Copyright (c) 2026 The SLUG developers. ISC license.
package fightertransfer

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestInspectCommitIndependentReference(t *testing.T) {
	c := readReference(t)
	for _, v := range c.Positives {
		t.Run(v.Name, func(t *testing.T) {
			expected, evidence := supplied(t, v)
			env, e := ParseEnvelope(unhex(t, v.Envelope))
			if e != nil {
				t.Fatal(e)
			}
			got, e := InspectCommit(context.Background(), expected.PinID, expected.PriorStateID, env.Intent.RecipientGym, evidence.CommitRaw, evidence.FundingTransactions)
			if e != nil {
				t.Fatal(e)
			}
			raw, e := MarshalIntent(got.Intent)
			if e != nil || !bytes.Equal(raw, unhex(t, v.Intent)) || got.CommitTxID.String() != v.CommitID {
				t.Fatal("independent commit reconstruction", e)
			}
			// The inspector must not pretend a gym authorization is present in a commit.
			changed := env
			changed.Signature = [64]byte{}
			if _, e = MarshalEnvelope(changed); e == nil {
				t.Fatal("invalid signature control")
			}
			if _, e = InspectCommit(context.Background(), expected.PinID, expected.PriorStateID, env.Intent.RecipientGym, evidence.CommitRaw, evidence.FundingTransactions); e != nil {
				t.Fatal("inspection improperly depends on unseen signature", e)
			}
			recipient := env.Intent.RecipientGym
			recipient[32] ^= 1
			if _, e = InspectCommit(context.Background(), expected.PinID, expected.PriorStateID, recipient, evidence.CommitRaw, evidence.FundingTransactions); e == nil {
				t.Fatal("foreign locator accepted")
			}
			if _, e = InspectCommit(context.Background(), expected.PinID, expected.PriorStateID, env.Intent.RecipientGym, evidence.CommitRaw, nil); !errors.Is(e, ErrUnavailable) {
				t.Fatal("missing funding parent", e)
			}
			rawCommit := append([]byte(nil), evidence.CommitRaw...)
			rawCommit[len(rawCommit)-5] ^= 1
			if _, e = InspectCommit(context.Background(), expected.PinID, expected.PriorStateID, env.Intent.RecipientGym, rawCommit, evidence.FundingTransactions); e == nil {
				t.Fatal("changed signed funding data accepted")
			}
		})
	}
	v := c.Positives[0]
	expected, evidence := supplied(t, v)
	env, _ := ParseEnvelope(unhex(t, v.Envelope))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := InspectCommit(ctx, expected.PinID, expected.PriorStateID, env.Intent.RecipientGym, evidence.CommitRaw, evidence.FundingTransactions); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
