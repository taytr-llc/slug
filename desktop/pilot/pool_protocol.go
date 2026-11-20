// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolsettle"
	"io"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type PoolRequest struct {
	ID         uint64            `json:"id"`
	Command    string            `json:"command"`
	Sequence   uint64            `json:"sequence,omitempty"`
	Path       string            `json:"path,omitempty"`
	Fixture    string            `json:"fixture,omitempty"`
	ViewID     string            `json:"view_id,omitempty"`
	ProspectID string            `json:"prospect_id,omitempty"`
	Selection  *recoveryfile.Pin `json:"selection,omitempty"`
}
type PoolResponse struct {
	ID              uint64            `json:"id"`
	OK              bool              `json:"ok"`
	Snapshot        *PoolSnapshot     `json:"snapshot,omitempty"`
	Portrait        *ManifestPortrait `json:"portrait,omitempty"`
	ManifestReceipt *ManifestReceipt  `json:"manifest_receipt,omitempty"`
	Error           string            `json:"error,omitempty"`
}

type PoolStableAPI interface {
	StableSnapshot() StableSnapshot
	BeginStableSync(context.Context, string) (StableJob, error)
	BeginStableRecent(context.Context, string) (StableJob, error)
	BeginStableInventory(context.Context, string, uint64, uint32) (StableJob, error)
	BeginStableRoster(context.Context, string, uint32, uint32) (StableJob, error)
	BeginStablePrepare(context.Context, string, gymstable.SlotRequest) (StableJob, error)
	BeginStableApply(context.Context, string, string) (StableJob, error)
	StableJob(context.Context, string, string) (StableJob, error)
	StableOperation(context.Context, string, string) (StableOperation, error)
	StablePending(context.Context, string) (StableOperation, error)
	AbandonStableOperation(context.Context, string, string) (StableOperation, error)
	CancelStableJob(context.Context, string, string) (StableJob, error)
}
type PoolSurfaceAPI interface {
	PoolStableAPI
	EntrySurfaceAPI
	EventSurfaceAPI
	SeedSurfaceAPI
	CardSurfaceAPI
	BoutSurfaceAPI
	ResultSurfaceAPI
	CareerSurfaceAPI
	CareerHistorySurfaceAPI
	Start() error
	Stop()
	Close() error
	Snapshot() PoolSnapshot
	RefreshPoolBalance(context.Context) error
	ManifestPortrait(context.Context, uint64) (ManifestPortrait, error)
	ManifestReceipt(context.Context, uint64) (ManifestReceipt, error)
	ExportManifestReceipt(context.Context, uint64, string, string, string) error
	ReleaseManifestReceipt(context.Context, string, string) error
}
type PoolChildAPI interface {
	PoolSurfaceAPI
	AccountID() string
	Prepare(context.Context) error
	ExportPoolRecovery(context.Context, recoveryfile.Pin) error
	VerifyPoolBackup(context.Context, recoveryfile.Pin) error
	CancelPoolBackup()
}
type PoolControllerAPI interface {
	PoolSurfaceAPI
	LoginFixture(string) error
	Logout() error
	ResetFixtureAccount() error
	ExportPoolRecovery(context.Context, string, recoveryfile.Pin) error
	VerifyPoolBackup(context.Context, string, recoveryfile.Pin) error
	CancelPoolBackup(context.Context, string) error
}

func ParsePoolRequest(raw []byte) (PoolRequest, error) {
	var req PoolRequest
	invalid := errors.New("invalid_request")
	if len(raw) == 0 || len(raw) > MaxRequestSize || !utf8.Valid(raw) {
		return req, invalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return req, invalid
	}
	fields := make(map[string]json.RawMessage, 7)
	for d.More() {
		token, err := d.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return req, invalid
		}
		if _, exists := fields[name]; exists {
			return req, invalid
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return req, invalid
		}
		fields[name] = value
	}
	if _, err := d.Token(); err != nil || d.Decode(new(any)) != io.EOF {
		return req, invalid
	}
	if json.Unmarshal(fields["id"], &req.ID) != nil || req.ID == 0 || req.ID > 1<<53-1 || json.Unmarshal(fields["command"], &req.Command) != nil {
		return req, invalid
	}
	for name := range fields {
		switch name {
		case "id", "command", "sequence", "path", "fixture", "view_id", "prospect_id", "selection":
		default:
			return req, invalid
		}
	}
	allowed := map[string]bool{"id": true, "command": true}
	sequence := false
	switch req.Command {
	case "start", "stop", "snapshot", "logout", "reset_fixture", "refresh_pool_balance":
	case "login_fixture":
		allowed["fixture"] = true
	case "manifest_portrait", "manifest_receipt":
		sequence = true
	case "export_manifest_receipt":
		sequence = true
		allowed["path"], allowed["view_id"], allowed["prospect_id"] = true, true, true
	case "release_manifest_receipt":
		allowed["view_id"], allowed["prospect_id"] = true, true
	case "export_recovery", "verify_backup":
		allowed["view_id"], allowed["selection"] = true, true
	case "cancel_pool_backup":
		allowed["view_id"] = true
	default:
		return req, invalid
	}
	if sequence {
		allowed["sequence"] = true
	}
	for name := range fields {
		if !allowed[name] {
			return req, invalid
		}
	}
	if sequence && (json.Unmarshal(fields["sequence"], &req.Sequence) != nil || req.Sequence == 0 || req.Sequence > MaxManifestSequence) {
		return req, invalid
	}
	if allowed["path"] && (json.Unmarshal(fields["path"], &req.Path) != nil || !ValidRecoveryPath(req.Path)) {
		return req, invalid
	}
	if allowed["fixture"] && (json.Unmarshal(fields["fixture"], &req.Fixture) != nil || (req.Fixture != "zero256" && req.Fixture != "incrementing256")) {
		return req, invalid
	}
	if allowed["view_id"] && (json.Unmarshal(fields["view_id"], &req.ViewID) != nil || !canonicalManifestID(req.ViewID)) {
		return req, invalid
	}
	if allowed["prospect_id"] && (json.Unmarshal(fields["prospect_id"], &req.ProspectID) != nil || !canonicalManifestID(req.ProspectID)) {
		return req, invalid
	}
	if allowed["selection"] {
		var pin recoveryfile.Pin
		if _, e := poolDecode(fields["selection"], MaxRequestSize, 6, &pin); e != nil {
			return req, invalid
		}
		mode := recoveryfile.CreateMode
		if req.Command == "verify_backup" {
			mode = recoveryfile.ReadMode
		}
		if recoveryfile.ValidatePin(pin, mode) != nil {
			return req, invalid
		}
		req.Selection = &pin
	}
	return req, nil
}

func handlePoolRequest(ctx context.Context, req PoolRequest, c PoolControllerAPI) PoolResponse {
	result := PoolResponse{ID: req.ID, OK: true}
	var err error
	code := "account_unavailable"
	switch req.Command {
	case "snapshot":
	case "start":
		err, code = c.Start(), "start_unavailable"
	case "stop":
		c.Stop()
	case "login_fixture":
		err = c.LoginFixture(req.Fixture)
	case "logout":
		err = c.Logout()
	case "reset_fixture":
		err = c.ResetFixtureAccount()
	case "refresh_pool_balance":
		err, code = c.RefreshPoolBalance(ctx), "balance_unavailable"
	case "export_recovery":
		err, code = c.ExportPoolRecovery(ctx, req.ViewID, *req.Selection), "export_unavailable"
		if errors.Is(err, recoveryfile.ErrOutcomeUnknown) {
			code = "export_uncertain"
		}
	case "verify_backup":
		err, code = c.VerifyPoolBackup(ctx, req.ViewID, *req.Selection), "backup_unavailable"
	case "cancel_pool_backup":
		err, code = c.CancelPoolBackup(ctx, req.ViewID), "backup_unavailable"
	case "manifest_portrait":
		value, e := c.ManifestPortrait(ctx, req.Sequence)
		err, code = e, "portrait_unavailable"
		if e == nil {
			result.Portrait = &value
		}
	case "manifest_receipt":
		value, e := c.ManifestReceipt(ctx, req.Sequence)
		err, code = e, "manifest_receipt_unavailable"
		if e == nil {
			result.ManifestReceipt = &value
		}
	case "export_manifest_receipt":
		err, code = c.ExportManifestReceipt(ctx, req.Sequence, req.ViewID, req.ProspectID, req.Path), "export_unavailable"
	case "release_manifest_receipt":
		err, code = c.ReleaseManifestReceipt(ctx, req.ViewID, req.ProspectID), "manifest_receipt_unavailable"
	default:
		err, code = errors.New("invalid_request"), "invalid_request"
	}
	if err != nil {
		return PoolResponse{ID: req.ID, Error: manifestProtocolError(err, code)}
	}
	if result.Portrait == nil && result.ManifestReceipt == nil {
		value := c.Snapshot()
		result.Snapshot = &value
	}
	return result
}

func handlePoolStableRequest(ctx context.Context, r StableRequest, c PoolControllerAPI) StableResponse {
	result := StableResponse{ID: r.ID, OK: true}
	var err error
	switch r.Command {
	case "stable_snapshot":
		v := c.StableSnapshot()
		result.Snapshot = &v
	case "stable_recent":
		v, e := c.BeginStableRecent(ctx, r.ViewID)
		result.Job, err = &v, e
	case "stable_sync":
		v, e := c.BeginStableSync(ctx, r.ViewID)
		result.Job, err = &v, e
	case "stable_inventory":
		v, e := c.BeginStableInventory(ctx, r.ViewID, r.After, r.Limit)
		result.Job, err = &v, e
	case "stable_roster":
		v, e := c.BeginStableRoster(ctx, r.ViewID, r.FirstSlot, r.Limit)
		result.Job, err = &v, e
	case "stable_prepare_slot":
		v, e := c.BeginStablePrepare(ctx, r.ViewID, gymstable.SlotRequest{ExpectedRevision: r.ExpectedRevision, Slot: r.Slot, ProspectID: r.ProspectID})
		result.Job, err = &v, e
	case "stable_apply_slot":
		v, e := c.BeginStableApply(ctx, r.ViewID, r.OperationID)
		result.Job, err = &v, e
	case "stable_job":
		v, e := c.StableJob(ctx, r.ViewID, r.JobID)
		result.Job, err = &v, e
	case "stable_operation":
		v, e := c.StableOperation(ctx, r.ViewID, r.OperationID)
		result.Operation, err = &v, e
	case "stable_pending":
		v, e := c.StablePending(ctx, r.ViewID)
		result.Operation, err = &v, e
	case "stable_abandon_operation":
		v, e := c.AbandonStableOperation(ctx, r.ViewID, r.OperationID)
		result.Operation, err = &v, e
	case "stable_cancel":
		v, e := c.CancelStableJob(ctx, r.ViewID, r.JobID)
		result.Job, err = &v, e
	default:
		err = gymstable.ErrInvalid
	}
	if err != nil {
		return StableResponse{ID: r.ID, Error: stableProtocolError(err)}
	}
	return result
}

func poolFraction(f poolsettle.Fraction) (*big.Rat, bool) {
	if len(f.Numerator) == 0 || len(f.Numerator) > 4096 || len(f.Denominator) == 0 || len(f.Denominator) > 4096 {
		return nil, false
	}
	n, nok := new(big.Int).SetString(f.Numerator, 10)
	d, dok := new(big.Int).SetString(f.Denominator, 10)
	if !nok || !dok || n.Sign() < 0 || d.Sign() <= 0 || n.String() != f.Numerator || d.String() != f.Denominator {
		return nil, false
	}
	r := new(big.Rat).SetFrac(n, d)
	return r, r.Num().String() == f.Numerator && r.Denom().String() == f.Denominator && r.Cmp(new(big.Rat).SetInt64(poolsettle.MaxMoney)) <= 0
}
func poolBalanceValid(s PoolSnapshot) bool {
	b := s.Balance
	if !stableOne(b.State, "unavailable", "loading", "ready", "failed") {
		return false
	}
	if b.State != "ready" {
		return b.Value == nil && ((b.State == "failed" && b.Error == "balance_unavailable") || (b.State != "failed" && b.Error == ""))
	}
	if b.Error != "" || b.Value == nil {
		return false
	}
	v := b.Value
	r := v.Balance
	if v.Version != 1 || v.BindingID != s.SourceBindingID || v.ObservedAtMS < 0 || v.ObservedAtMS > 8640000000000000 || r.GymID != s.GymID || r.PaidAtoms < 0 || r.PaidAtoms > poolsettle.MaxMoney || r.ReservedAtoms < 0 || r.ReservedAtoms > poolsettle.MaxMoney {
		return false
	}
	if !stableOne(r.Reason, "", "payout-pending", "broadcast-unknown", "transaction-unavailable", "payout-conflict", "funding-unavailable", "funding-conflict", "funding-changed", "immature", "awaiting-confirmation", "below-minimum", "destination-unavailable", "daily-batch-complete", "fee-unavailable", "fee-budget", "change-dust", "signing-unavailable", "capacity") {
		return false
	}
	confirmed, ok := poolFraction(r.Confirmed)
	if !ok {
		return false
	}
	available, ok := poolFraction(r.Available)
	if !ok {
		return false
	}
	remaining := new(big.Rat).Sub(confirmed, new(big.Rat).SetInt64(r.ReservedAtoms))
	return remaining.Sign() >= 0 && available.Cmp(remaining) <= 0 && (confirmed.Sign() != 0 || r.Reason == "") && (v.ObservedAtMS != 0 || (confirmed.Sign() == 0 && available.Sign() == 0 && r.PaidAtoms == 0 && r.ReservedAtoms == 0 && r.Reason == ""))
}
func PoolStatusCode(s string) bool {
	return stableOne(s, "", "pool_unavailable", "pool_shutdown_failed", "pool_capacity", "balance_unavailable", "initialization_failed", "storage_failed", "storage_close_failed", "chain_data_missing", "node_start_failed", "node_shutdown_failed", "wallet_unavailable", "wallet_shutdown_failed", "wallet_connection_failed", "wallet_payout_failed", "wallet_identity_mismatch", "wallet_status_failed", "node_unavailable", "authority_unavailable", "authority_identity_mismatch", "authority_port_busy", "wallet_signer_unavailable", "worker_unavailable", "worker_failed", "node_exited", "authority_exited", "observer_unavailable", "signer_unavailable", "clock", "stale", "capacity", "unavailable", "outcome_unknown", "unauthorized", "forbidden", "rejected", "unknown_job", "busy", "rate_limited", "expired", "invalid_request", "too_large")
}

var poolPayoutPattern = regexp.MustCompile(`^slug1q[023456789acdefghjklmnpqrstuvwxyz]{38}$`)

// ValidatePoolSnapshot is structural, bounded, and performs no proof or RPC work.
func ValidatePoolSnapshot(s PoolSnapshot) error {
	bad := gymstable.ErrInvalid
	a := s.Account
	if s.Version != 1 || !stableOne(s.Mode, PoolMode, HostedPoolMode, HostedRecoveryPoolMode) || !stableOne(s.State, "stopped", "starting", "running", "stopping", "failed") || !stableOne(s.WalletState, "closed", "connecting", "syncing", "ready", "failed") || !poolBackupValid(s) || !PoolStatusCode(s.LastError) || !PoolStatusCode(s.RecoveryError) {
		return bad
	}
	if s.Attempts > gymstable.MaxSafeInteger || s.Height > gymstable.MaxSafeInteger || s.WalletHeight > gymstable.MaxSafeInteger || s.Pending > 1000 || s.AssignmentCount > 1000 || s.SpendableAtoms < 0 || s.SpendableAtoms > poolsettle.MaxMoney || s.ImmatureAtoms < 0 || s.ImmatureAtoms > poolsettle.MaxMoney || math.IsNaN(s.HashesPerSecond) || math.IsInf(s.HashesPerSecond, 0) || s.HashesPerSecond < 0 {
		return bad
	}
	pendingReset := stableOne(s.Mode, HostedPoolMode, HostedRecoveryPoolMode) && a.LastError == "hosted_reset_pending" && stableOne(a.State, "signed_out", "failed") && stableOne(a.Fixture, "zero256", "incrementing256")
	if !stableOne(a.State, "signed_out", "signing_in", "signed_in", "resetting", "signing_out", "failed", "expired") || !stableOne(a.Fixture, "", "zero256", "incrementing256") || !(stableOne(a.LastError, "", "account_unavailable") || pendingReset) || a.Generation > gymstable.MaxSafeInteger || a.HoldUntilMS < 0 || a.HoldUntilMS > 8640000000000000 || a.Held && (a.CanExport || a.HoldUntilMS == 0) {
		return bad
	}
	if !poolBalanceValid(s) {
		return bad
	}
	if a.State != "signed_in" {
		if s.View != nil || a.ID != "" || a.Generation != 0 || a.HoldUntilMS != 0 || a.Held || a.CanExport || s.BackupState != "unavailable" || s.GymID != "" || s.GymPublicKey != "" || s.PayoutAddress != "" || s.SourceBindingID != "" || s.AssignmentCount != 0 || s.SpendableAtoms != 0 || s.ImmatureAtoms != 0 || s.Pending != 0 || s.Attempts != 0 || s.Height != 0 || s.WalletHeight != 0 || s.HashesPerSecond != 0 || s.Balance.State != "unavailable" {
			return bad
		}
		return nil
	}
	identity, e := gymaccount.FixtureIdentity(a.Fixture)
	if e != nil || s.GymID != identity.GymID || s.GymPublicKey != identity.GymPublicKey || !canonicalManifestID(a.ID) || a.Generation == 0 || !canonicalManifestID(s.SourceBindingID) || !poolPayoutPattern.MatchString(s.PayoutAddress) || s.View == nil || !stableViewValid(*s.View) || s.View.AccountID != a.ID || s.View.Generation != a.Generation {
		return bad
	}
	return nil
}
func ParsePoolResponse(raw []byte) (PoolResponse, error) {
	var r PoolResponse
	fields, e := poolDecode(raw, MaxResponseSize-1, 12, &r)
	if e != nil || r.ID == 0 || r.ID > gymstable.MaxSafeInteger || len(fields) != 3 || !fields["id"] || !fields["ok"] {
		return PoolResponse{}, gymstable.ErrInvalid
	}
	canonical, e := json.Marshal(r)
	if e != nil || !bytes.Equal(raw, canonical) {
		return PoolResponse{}, gymstable.ErrInvalid
	}
	if !r.OK {
		if !fields["error"] || !stableOne(r.Error, "invalid_request", "invalid_frame", "start_unavailable", "account_unavailable", "balance_unavailable", "portrait_unavailable", "manifest_receipt_unavailable", "export_unavailable", "export_uncertain", "backup_unavailable", "initialization_incomplete", "projection_unavailable", "projection_corrupt", "projection_cancelled", "projection_required", "prospect_unavailable") {
			return PoolResponse{}, gymstable.ErrInvalid
		}
		return r, nil
	}
	valid := false
	switch {
	case fields["snapshot"]:
		valid = r.Snapshot != nil && ValidatePoolSnapshot(*r.Snapshot) == nil
	case fields["portrait"]:
		valid = r.Portrait != nil && r.Portrait.Version == 1 && stableViewValid(r.Portrait.View) && r.Portrait.Sequence > 0 && r.Portrait.Sequence <= 1000 && len(r.Portrait.SVG) > 0 && len(r.Portrait.SVG) <= 32768
	case fields["manifest_receipt"]:
		valid = r.ManifestReceipt != nil && r.ManifestReceipt.Version == 1 && stableViewValid(r.ManifestReceipt.View) && r.ManifestReceipt.Record.Sequence > 0 && r.ManifestReceipt.Record.Sequence <= 1000
	}
	if !valid {
		return PoolResponse{}, gymstable.ErrInvalid
	}
	return r, nil
}
func respondPool(w io.Writer, r PoolResponse) error {
	budget := MaxResponseSize - 1
	if !manifestResponseBudget(reflect.ValueOf(r), &budget, 0) {
		return gymstable.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if _, e = ParsePoolResponse(raw); e != nil {
		return e
	}
	return respondPoolFamily(w, raw)
}

// Family marshaling and bounds are checked before this common pipe write.
func respondPoolFamily(w io.Writer, raw []byte) error {
	if d, ok := w.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = d.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	raw = append(raw, '\n')
	n, e := w.Write(raw)
	if e != nil {
		return e
	}
	if n != len(raw) {
		return io.ErrShortWrite
	}
	return nil
}
func poolDecode(raw []byte, limit, maxDepth int, out any) (map[string]bool, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return nil, gymstable.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	root := map[string]bool{}
	var walk func(reflect.Type, string, int) error
	walk = func(t reflect.Type, path string, depth int) error {
		token, err := d.Token()
		if err != nil {
			return gymstable.ErrInvalid
		}
		if t.Kind() == reflect.Pointer {
			if token == nil {
				return nil
			}
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Struct:
			if token != json.Delim('{') || depth+1 > maxDepth {
				return gymstable.ErrInvalid
			}
			names := map[string]reflect.StructField{}
			required := map[string]bool{}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.PkgPath != "" {
					continue
				}
				tag := strings.Split(f.Tag.Get("json"), ",")
				if tag[0] == "-" {
					continue
				}
				names[tag[0]] = f
				if len(tag) == 1 || tag[1] != "omitempty" {
					required[tag[0]] = true
				}
			}
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				name, ok := key.(string)
				f, known := names[name]
				if e != nil || !ok || len(name) > 64 || !known || seen[name] {
					return gymstable.ErrInvalid
				}
				seen[name] = true
				next := name
				if path != "" {
					next = path + "." + name
				}
				if e = walk(f.Type, next, depth+1); e != nil {
					return e
				}
			}
			for name := range required {
				if !seen[name] {
					return gymstable.ErrInvalid
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return gymstable.ErrInvalid
			}
			if path == "" {
				root = seen
			}
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') || depth+1 > maxDepth {
				return gymstable.ErrInvalid
			}
			count := 0
			bound := 20
			if t.Kind() == reflect.Array {
				bound = t.Len()
			}
			for d.More() {
				if count >= bound {
					return gymstable.ErrInvalid
				}
				if e := walk(t.Elem(), path+"[]", depth+1); e != nil {
					return e
				}
				count++
			}
			if t.Kind() == reflect.Array && count != t.Len() {
				return gymstable.ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return gymstable.ErrInvalid
			}
		case reflect.String:
			s, ok := token.(string)
			if !ok || len(s) > 73728 {
				return gymstable.ErrInvalid
			}
		case reflect.Bool:
			if _, ok := token.(bool); !ok {
				return gymstable.ErrInvalid
			}
		case reflect.Int, reflect.Int32, reflect.Int64:
			n, ok := token.(json.Number)
			if !ok {
				return gymstable.ErrInvalid
			}
			value, e := strconv.ParseInt(string(n), 10, t.Bits())
			signed := strings.HasSuffix(path, ".shoulder_jitter")
			if e != nil || strconv.FormatInt(value, 10) != string(n) || value > int64(gymstable.MaxSafeInteger) || value < 0 && (!signed || value < -6) || signed && value > 6 {
				return gymstable.ErrInvalid
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			n, ok := token.(json.Number)
			if !ok {
				return gymstable.ErrInvalid
			}
			value, e := strconv.ParseUint(string(n), 10, t.Bits())
			if e != nil || strconv.FormatUint(value, 10) != string(n) || value > gymstable.MaxSafeInteger {
				return gymstable.ErrInvalid
			}
		case reflect.Float64:
			n, ok := token.(json.Number)
			if !ok {
				return gymstable.ErrInvalid
			}
			v, e := strconv.ParseFloat(string(n), 64)
			if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
				return gymstable.ErrInvalid
			}
		default:
			return gymstable.ErrInvalid
		}
		return nil
	}
	if err := walk(reflect.TypeOf(out).Elem(), "", 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, gymstable.ErrInvalid
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, gymstable.ErrInvalid
	}
	return root, nil
}
func ServePool(ctx context.Context, input io.Reader, output io.Writer, c PoolControllerAPI) (resultErr error) {
	return servePool(ctx, input, output, c, nil)
}

func servePool(ctx context.Context, input io.Reader, output io.Writer, c PoolControllerAPI, hosted HostedRecoverySurfaceAPI) (resultErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type line struct {
		data []byte
		err  error
	}
	lines := make(chan line, 8)
	readerDone, shutdownDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
		case <-readerDone:
		}
		c.Stop()
		if x, ok := input.(io.Closer); ok {
			_ = x.Close()
		}
		if x, ok := output.(io.Closer); ok {
			_ = x.Close()
		}
		_ = c.Close()
	}()
	defer func() { cancel(); <-shutdownDone; resultErr = errors.Join(resultErr, c.Close()) }()
	go func() {
		defer close(readerDone)
		defer close(lines)
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 1024), max(MaxRequestSize, MaxEntryRequestSize, MaxEventRequestSize, MaxSeedRequestSize, MaxCardRequestSize, MaxBoutRequestSize, MaxResultRequestSize, MaxCareerRequestSize, MaxCareerHistoryRequestSize)+1)
		for scanner.Scan() {
			value := line{data: append([]byte(nil), scanner.Bytes()...)}
			select {
			case lines <- value:
			case <-ctx.Done():
				return
			default:
				return
			}
		}
		select {
		case lines <- line{err: scanner.Err()}:
		case <-ctx.Done():
		default:
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case value, open := <-lines:
			if !open || value.data == nil {
				return value.err
			}
			var selector struct {
				ID      uint64 `json:"id"`
				Command string `json:"command"`
			}
			if json.Unmarshal(value.data, &selector) != nil {
				return gymstable.ErrInvalid
			}
			if strings.HasPrefix(selector.Command, "hosted_recovery_") && hosted != nil {
				response := HostedRecoveryResponse{ID: selector.ID, Version: 1, Error: "invalid_request"}
				if req, e := ParseHostedRecoveryRequest(value.data); e == nil {
					response = HandleHostedRecoveryRequest(ctx, req, hosted)
				}
				raw, e := MarshalHostedRecoveryResponse(response)
				if e != nil {
					return e
				}
				if e = respondPoolFamily(output, raw); e != nil {
					return e
				}
			} else if raw, handled, err := poolCycleResponse(ctx, value.data, selector.ID, selector.Command, c); handled {
				if err != nil {
					return err
				}
				if err = respondPoolFamily(output, raw); err != nil {
					return err
				}
			} else if strings.HasPrefix(selector.Command, "entry_") || strings.HasPrefix(selector.Command, "event_") {
				if selector.ID == 0 || selector.ID > gymstable.MaxSafeInteger {
					return gymstable.ErrInvalid
				}
				var raw []byte
				var err error
				if strings.HasPrefix(selector.Command, "entry_") {
					req, parseErr := ParseEntryRequest(value.data)
					response := EntryResponse{ID: selector.ID, Error: "invalid_request"}
					if parseErr == nil {
						response = handleEntryRequest(ctx, req, c)
					}
					raw, err = MarshalEntryResponse(response)
				} else {
					req, parseErr := ParseEventRequest(value.data)
					response := EventResponse{ID: selector.ID, Error: "invalid_request"}
					if parseErr == nil {
						response = handleEventRequest(ctx, req, c)
					}
					raw, err = MarshalEventResponse(response)
				}
				if err != nil {
					return err
				}
				if err = respondPoolFamily(output, raw); err != nil {
					return err
				}
			} else if strings.HasPrefix(selector.Command, "stable_") {
				req, err := ParseStableRequest(value.data)
				var response StableResponse
				if err != nil {
					if selector.ID == 0 || selector.ID > gymstable.MaxSafeInteger {
						return gymstable.ErrInvalid
					}
					response = StableResponse{ID: selector.ID, Error: "invalid_request"}
				} else {
					response = handlePoolStableRequest(ctx, req, c)
				}
				if err = respondStable(output, response); err != nil {
					return err
				}
			} else {
				req, err := ParsePoolRequest(value.data)
				var response PoolResponse
				if err != nil {
					response = PoolResponse{ID: req.ID, Error: "invalid_request"}
				} else {
					if hosted != nil && stableOne(req.Command, "snapshot", "start", "stop", "login_fixture", "logout", "reset_fixture", "export_recovery", "verify_backup", "cancel_pool_backup") {
						response = PoolResponse{ID: req.ID, Error: "invalid_request"}
					} else {
						response = handlePoolRequest(ctx, req, c)
					}
				}
				if err = respondPool(output, response); err != nil {
					return err
				}
			}
		}
	}
}
