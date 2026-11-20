// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"context"
	"errors"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestSharedPublisherProfileAndZeroRefusal(t *testing.T) {
	for _, result := range []bool{false, true} {
		var err error
		if result {
			_, err = AttachSharedPoolResult(context.Background(), SharedPoolOwnerBinding{})
		} else {
			_, err = AttachSharedPool(context.Background(), SharedPoolOwnerBinding{})
		}
		if wire.TestPoW && !errors.Is(err, ErrProfile) || !wire.TestPoW && !errors.Is(err, ErrInvalid) {
			t.Fatal("unowned/profile-refused shared attachment", result, err)
		}
	}
}
