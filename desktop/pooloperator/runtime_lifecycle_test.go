// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestOperatorRuntimeCompletionClassification(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	unexpected := errors.New("synthetic service failure")
	for _, tc := range []struct {
		name       string
		ctx        context.Context
		cause      error
		cancelOnly bool
		retained   error
	}{
		{"active nil completion", context.Background(), nil, false, ErrUnavailable},
		{"active unexpected cancellation", context.Background(), context.Canceled, false, ErrUnavailable},
		{"canceled nil completion", canceled, nil, true, context.Canceled},
		{"canceled leaf", canceled, context.Canceled, true, context.Canceled},
		{"canceled wrapped leaf", canceled, fmt.Errorf("joined task: %w", context.Canceled), true, context.Canceled},
		{"canceled siblings", canceled, errors.Join(context.Canceled, context.Canceled), true, context.Canceled},
		{"canceled cleanup failure", canceled, errors.Join(context.Canceled, ErrCleanup), false, ErrCleanup},
		{"canceled service failure", canceled, errors.Join(context.Canceled, unexpected), false, unexpected},
		{"canceled unknown action", canceled, errors.Join(context.Canceled, ErrActionOutcomeUnknown), false, ErrActionOutcomeUnknown},
		{"canceled earlier action deadline", canceled, errors.Join(context.Canceled, context.DeadlineExceeded), false, context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runtimeCompletion(tc.ctx, tc.cause)
			if runtimeCancelledOnly(got) != tc.cancelOnly || !errors.Is(got, tc.retained) {
				t.Fatalf("lost completion category: %v", got)
			}
			if tc.cancelOnly && errors.Is(got, ErrUnavailable) {
				t.Fatal("normal cancellation labeled unavailable")
			}
		})
	}
}
