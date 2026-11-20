// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/walletdb"
)

func TestOperatorWalletDriverRegistered(t *testing.T) {
	for _, name := range walletdb.SupportedDrivers() {
		if name == "bdb" {
			return
		}
	}
	t.Fatal("standalone operator cannot open its dedicated wallet")
}

func TestOperatorPublicMaintenanceContention(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		retry bool
	}{
		{"owner busy", operatorauthority.ErrBusy, true},
		{"wrapped owner busy", fmt.Errorf("publication: %w", operatorauthority.ErrBusy), true},
		{"joined known retry", errors.Join(operatorauthority.ErrBusy, registry.ErrBusy, context.DeadlineExceeded), true},
		{"stale replay", fighterownership.ErrStale, true},
		{"owner closed", operatorauthority.ErrClosed, false},
		{"owner integrity", operatorauthority.ErrIntegrity, false},
		{"owner binding", operatorauthority.ErrBinding, false},
		{"busy with closed", errors.Join(operatorauthority.ErrBusy, operatorauthority.ErrClosed), false},
		{"busy with cleanup", errors.Join(operatorauthority.ErrBusy, ErrCleanup), false},
		{"stale with cleanup", errors.Join(fighterownership.ErrStale, ErrCleanup), false},
		{"canceled", context.Canceled, false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if publicRetry(tc.cause) != tc.retry {
				t.Fatal("wrong maintenance retry classification", tc.cause)
			}
		})
	}
	if maintenanceCode(operatorauthority.ErrBusy) != "busy" {
		t.Fatal("ordinary owner contention not visible as busy")
	}
}

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
