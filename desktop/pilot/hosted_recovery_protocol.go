// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

var (
	ErrHostedRecoveryUnavailable = errors.New("hosted recovery unavailable")
	ErrHostedWalletMissing       = errors.New("hosted wallet missing")
	ErrHostedStaleView           = errors.New("hosted recovery stale view")
)

type HostedRecoveryControllerAPI interface {
	PoolControllerAPI
	HostedRecoverySurfaceAPI
}

func ServeHostedRecovery(ctx context.Context, input io.Reader, output io.Writer, c HostedRecoveryControllerAPI) error {
	if c == nil || c.HostedRecoverySnapshot().Mode != HostedRecoveryPoolMode {
		return gymstable.ErrInvalid
	}
	return servePool(ctx, input, output, c, c)
}
func HostedRecoveryError(err error) string {
	var backup *hostedBackupError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, recoveryvault.ErrExportUncertain):
		return "backup_export_uncertain"
	case errors.Is(err, wallet.ErrAllocationOutcomeUnknown), errors.Is(err, recoveryvault.ErrOutcomeUnknown):
		return "outcome_unknown"
	case errors.Is(err, recoveryvault.ErrCleanup):
		return "backup_storage_failed"
	case errors.As(err, &backup):
		return backup.code
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, gymaccount.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, gymaccount.ErrForbidden), errors.Is(err, gymaccount.ErrHeld):
		return "forbidden"
	case errors.Is(err, ErrHostedStaleView):
		return "stale_view"
	case errors.Is(err, ErrHostedWalletMissing):
		return "wallet_missing"
	case errors.Is(err, wallet.ErrAllocationInvalid), errors.Is(err, gymstable.ErrInvalid):
		return "invalid_request"
	case errors.Is(err, wallet.ErrAllocationBusy):
		return "busy"
	case errors.Is(err, wallet.ErrAllocationCapacity):
		return "allocation_capacity"
	case errors.Is(err, wallet.ErrAllocationConflict):
		return "allocation_conflict"
	case errors.Is(err, wallet.ErrAllocationNotFound):
		return "allocation_not_found"
	case errors.Is(err, wallet.ErrAllocationResumeRequired):
		return "allocation_resume_required"
	case errors.Is(err, wallet.ErrAllocationOutcomeUnknown):
		return "outcome_unknown"
	case errors.Is(err, wallet.ErrAllocationUnavailable), errors.Is(err, wallet.ErrAllocationClosed), errors.Is(err, wallet.ErrAllocationPoisoned), errors.Is(err, wallet.ErrAllocationStorage), errors.Is(err, wallet.ErrAllocationCorrupt):
		return "allocation_unavailable"
	default:
		return "hosted_recovery_unavailable"
	}
}
func HandleHostedRecoveryRequest(ctx context.Context, r HostedRecoveryRequest, c HostedRecoverySurfaceAPI) HostedRecoveryResponse {
	response := HostedRecoveryResponse{ID: r.ID, Version: 1, Error: "invalid_request"}
	if !hostedRequestValid(r) || c == nil {
		return response
	}
	if c.HostedRecoverySnapshot().Mode != HostedRecoveryPoolMode {
		return response
	}
	var err error
	var job HostedReceiveJob
	switch strings.TrimPrefix(r.Command, "hosted_recovery_") {
	case "snapshot":
	case "login_fixture":
		err = c.LoginFixture(r.Fixture)
	case "logout":
		err = c.Logout()
	case "reset_fixture":
		err = c.ResetFixtureAccount()
	case "start":
		err = c.StartHostedRecovery(ctx, r.ViewID)
	case "stop":
		err = c.StopHostedRecovery(ctx, r.ViewID)
	case "receive_begin":
		job, err = c.BeginHostedReceive(ctx, r.ViewID, r.ActionID, r.OperationID)
	case "receive_lookup":
		job, err = c.LookupHostedReceive(ctx, r.ViewID, r.ActionID, r.OperationID)
	case "receive_resume":
		job, err = c.ResumeHostedReceive(ctx, r.ViewID, r.ActionID, r.OperationID)
	case "receive_redisplay":
		job, err = c.RedisplayHostedReceive(ctx, r.ViewID, r.ActionID, r.OperationID)
	case "receive_cancel":
		job, err = c.CancelHostedReceive(ctx, r.ViewID, r.ActionID, r.OperationID)
	case "export_recovery":
		err = c.ExportHostedRecovery(ctx, r.OwnerKind, r.ViewID, *r.Selection)
	case "verify_backup":
		err = c.VerifyHostedBackup(ctx, r.OwnerKind, r.ViewID, *r.Selection)
	case "cancel_pool_backup":
		err = c.CancelHostedBackup(ctx, r.OwnerKind, r.ViewID)
	default:
		return response
	}
	if err != nil {
		response.Error = HostedRecoveryError(err)
		return response
	}
	response.Error = ""
	response.OK = true
	if strings.HasPrefix(r.Command, "hosted_recovery_receive_") {
		response.Receive = &job
	} else {
		s := c.HostedRecoverySnapshot()
		response.Snapshot = &s
	}
	if !hostedResponseValid(response) {
		return HostedRecoveryResponse{ID: r.ID, Version: 1, Error: "outcome_unknown"}
	}
	return response
}
