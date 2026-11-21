// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/registry"
)

const DailyWalletApplication = "SLUG-REMOTE-DAILY-WALLET-1"
const MaxDailyWalletRequestSize = 2048 // Includes the single LF.
const MaxDailyWalletResponseSize = 65536

var ErrDailyWalletProtocol = errors.New("daily wallet protocol unavailable")

type DailyWalletRequest struct {
	Version         uint32 `json:"version"`
	Application     string `json:"application"`
	ID              uint64 `json:"id,string"`
	Command         string `json:"command"`
	ConfigID        string `json:"config_id"`
	ViewID          string `json:"view_id"`
	Fixture         string `json:"fixture"`
	Sequence        uint64 `json:"sequence,string"`
	CaptureSequence uint64 `json:"capture_sequence,string"`
	BeforeBatch     uint64 `json:"before_batch,string"`
}
type DailyWalletBinding struct {
	View                    RemotePoolView `json:"view"`
	ConfigID                string         `json:"config_id"`
	SourceBindingID         string         `json:"source_binding_id"`
	SettlementConfigID      string         `json:"settlement_config_id"`
	RecipientScript         string         `json:"recipient_script"`
	IssueProfileID          string         `json:"issue_profile_id"`
	OriginalSourceBindingID string         `json:"original_source_binding_id"`
	WalletOwnerID           string         `json:"wallet_owner_id"`
	AllocationBindingID     string         `json:"allocation_binding_id"`
}
type DailyWalletJob struct {
	Sequence        uint64 `json:"sequence,string"`
	CaptureSequence uint64 `json:"capture_sequence,string"`
	BeforeBatch     uint64 `json:"before_batch,string"`
	State           string `json:"state"`
	Error           string `json:"error"`
}
type DailyWalletCapture struct {
	Sequence     uint64                     `json:"sequence,string"`
	Binding      DailyWalletBinding         `json:"binding"`
	CapturedAtMS int64                      `json:"captured_at_ms"`
	Stale        bool                       `json:"stale"`
	First        poolsettle.DailyPayoutPage `json:"first"`
	Current      poolsettle.DailyPayoutPage `json:"current"`
}
type DailyWalletRead struct {
	Job     DailyWalletJob      `json:"job"`
	Capture *DailyWalletCapture `json:"capture"`
}
type DailyWalletResponse struct {
	Version     uint32                  `json:"version"`
	Application string                  `json:"application"`
	ID          uint64                  `json:"id,string"`
	Command     string                  `json:"command"`
	ConfigID    string                  `json:"config_id"`
	ViewID      string                  `json:"view_id"`
	OK          bool                    `json:"ok"`
	Snapshot    *RemotePoolWireSnapshot `json:"snapshot"`
	Job         *DailyWalletJob         `json:"job"`
	Read        *DailyWalletRead        `json:"read"`
	Error       string                  `json:"error"`
}

// The selected helper forwards to its original concrete controller. Public
// projections never replace this owner or mint View/Mining authority.
type DailyWalletControllerAPI interface {
	DailyWalletSnapshot(context.Context) (RemotePoolWireSnapshot, error)
	LoginFixture(context.Context, string) error
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Logout(context.Context, string) error
	BeginDailyPayoutRead(context.Context, string, uint64, uint64, uint64) (RemoteDailyPayoutJob, error)
	DailyPayoutRead(context.Context, string, uint64) (RemoteDailyPayoutRead, error)
	CancelDailyPayoutRead(context.Context, string, uint64) error
	Close() error
}

// No unbounded containers or recursive DTOs enter this local wire. In
// particular a bare native uint64 may not accidentally become a JS number.
func dailyWalletBound(v reflect.Value, depth int, quoted bool, left *int) bool {
	if !v.IsValid() || depth > 14 || *left < 0 {
		return false
	}
	switch v.Kind() {
	case reflect.Pointer:
		return v.IsNil() || dailyWalletBound(v.Elem(), depth+1, quoted, left)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if f.PkgPath != "" || !dailyWalletBound(v.Field(i), depth+1, strings.Contains(f.Tag.Get("json"), ",string"), left) {
				return false
			}
		}
	case reflect.Slice:
		if v.IsNil() || v.Len() > poolsettle.MaxDailyPayoutSlots {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !dailyWalletBound(v.Index(i), depth+1, false, left) {
				return false
			}
		}
	case reflect.String:
		s := v.String()
		if len(s) > *left || !remoteWireASCII(s) {
			return false
		}
		*left -= len(s)
	case reflect.Uint32, reflect.Uint64:
		return quoted || v.Uint() <= remotePoolWireMaxSafe
	case reflect.Int64:
		return v.Int() >= 0 && (quoted || uint64(v.Int()) <= remotePoolWireMaxSafe)
	case reflect.Bool:
	default:
		return false
	}
	return true
}
func dailyWalletFits(v any, limit int) bool {
	left := limit
	if !dailyWalletBound(reflect.ValueOf(v), 0, false, &left) {
		return false
	}
	b, e := json.Marshal(v)
	return e == nil && len(b)+1 <= limit
}
func dailyWalletDecode(raw []byte, limit int, out any) error {
	if len(raw) == 0 || len(raw)+1 > limit {
		return ErrDailyWalletProtocol
	}
	for _, b := range raw {
		if b < 32 || b > 126 {
			return ErrDailyWalletProtocol
		}
	}
	switch out.(type) {
	case *DailyWalletRequest, *DailyWalletResponse:
	default:
		return ErrDailyWalletProtocol
	}
	if json.Unmarshal(raw, out) != nil || !dailyWalletFits(reflect.ValueOf(out).Elem().Interface(), limit) {
		return ErrDailyWalletProtocol
	}
	b, e := json.Marshal(out)
	if e != nil || !bytes.Equal(raw, b) {
		return ErrDailyWalletProtocol
	}
	return nil
}
func dailyWalletMoneyCommand(s string) bool {
	return stableOne(s, "daily_payout_begin", "daily_payout_read", "daily_payout_cancel")
}
func ValidateDailyWalletRequest(q DailyWalletRequest, c RecoverableRemotePoolConfig) error {
	id, e := RecoverableRemotePoolConfigID(c)
	if e != nil || !dailyWalletFits(q, MaxDailyWalletRequestSize) || q.Version != 1 || q.Application != DailyWalletApplication || q.ID == 0 || q.ConfigID != id || q.ViewID != "" && !remotePoolID(q.ViewID) {
		return ErrDailyWalletProtocol
	}
	if q.ID == 1 && (q.Command != "snapshot" || q.ViewID != "") {
		return ErrDailyWalletProtocol
	}
	if dailyWalletMoneyCommand(q.Command) {
		if !remotePoolID(q.ViewID) || q.Fixture != "" || q.Sequence == 0 {
			return ErrDailyWalletProtocol
		}
		if q.Command != "daily_payout_begin" && (q.CaptureSequence != 0 || q.BeforeBatch != 0) {
			return ErrDailyWalletProtocol
		}
		if q.Command == "daily_payout_begin" && ((q.CaptureSequence == 0) != (q.BeforeBatch == 0) || q.CaptureSequence >= q.Sequence || q.BeforeBatch > poolsettle.MaxDailyPayoutBatches+1) {
			return ErrDailyWalletProtocol
		}
		return nil
	}
	if q.Sequence != 0 || q.CaptureSequence != 0 || q.BeforeBatch != 0 {
		return ErrDailyWalletProtocol
	}
	switch q.Command {
	case "snapshot":
		if q.Fixture != "" {
			return ErrDailyWalletProtocol
		}
	case "login_fixture":
		if q.ViewID != "" || q.Fixture != c.Base.ExpectedLocal.Recipient.Config.Fixture {
			return ErrDailyWalletProtocol
		}
	case "start", "stop", "logout":
		if !remotePoolID(q.ViewID) || q.Fixture != "" {
			return ErrDailyWalletProtocol
		}
	default:
		return ErrDailyWalletProtocol
	}
	return nil
}
func MarshalDailyWalletRequest(q DailyWalletRequest, c RecoverableRemotePoolConfig) ([]byte, error) {
	if ValidateDailyWalletRequest(q, c) != nil {
		return nil, ErrDailyWalletProtocol
	}
	return json.Marshal(q)
}
func ParseDailyWalletRequest(raw []byte, c RecoverableRemotePoolConfig) (DailyWalletRequest, error) {
	var q DailyWalletRequest
	if dailyWalletDecode(raw, MaxDailyWalletRequestSize, &q) != nil || ValidateDailyWalletRequest(q, c) != nil {
		return DailyWalletRequest{}, ErrDailyWalletProtocol
	}
	return q, nil
}

func ValidateDailyWalletSnapshot(s RemotePoolWireSnapshot, c RecoverableRemotePoolConfig) error {
	id, e := RecoverableRemotePoolConfigID(c)
	expected := c.Base
	if e != nil || !dailyWalletFits(s, MaxDailyWalletResponseSize) || s.Version != 2 || s.ConfigID != id {
		return ErrDailyWalletProtocol
	}
	a := s.Account
	if !stableOne(s.State, "signed_out", "signed_in", "failed", "expired") || a.State != s.State || a.Fixture != expected.ExpectedLocal.Recipient.Config.Fixture || a.CanExport || s.Error != a.LastError {
		return ErrDailyWalletProtocol
	}
	if s.State != "signed_in" {
		if s.View != nil || s.Remote != nil || a.ID != "" || a.Generation != 0 || a.HoldUntilMS != 0 || a.Held {
			return ErrDailyWalletProtocol
		}
		if s.State == "signed_out" && s.Error != "" || s.State == "failed" && s.Error != "remote_unavailable" || s.State == "expired" && !stableOne(s.Error, "", "remote_unavailable") {
			return ErrDailyWalletProtocol
		}
		return nil
	}
	if s.Error != "" || s.View == nil || s.Remote == nil || !remoteWireViewValid(*s.View, expected) || a.ID != s.View.AccountID || a.Generation != s.View.Generation || a.Held && a.HoldUntilMS == 0 {
		return ErrDailyWalletProtocol
	}
	r := s.Remote
	if r.Version != 2 || r.ConfigID != id || r.View != *s.View || r.SourceBindingID != expected.Miners.SourceID || r.OriginalSourceBindingID != expected.ExpectedLocal.OriginalSourceBindingID || r.PayoutAddress != expected.ExpectedLocal.Recipient.Recipient.Address || !stableOne(r.State, "stopped", "starting", "running", "failed") || remotePoolWorkerState(r.WorkerState) != r.WorkerState || r.WorkerState == "" || remotePoolPublicCode(r.LastError) != r.LastError || r.State != "running" && r.HashesPerSecondMilli != 0 || r.Records > expected.MaxRecords || r.Pending > r.Records || !stableOne(r.WalletState, "unavailable", "syncing", "ready") || r.WalletHeight > 2147483647 || r.SpendableAtoms > remotePoolWireMaxAtoms || r.ImmatureAtoms > remotePoolWireMaxAtoms || r.UnconfirmedAtoms > remotePoolWireMaxAtoms {
		return ErrDailyWalletProtocol
	}
	if r.WalletState == "unavailable" && (r.WalletHeight != 0 || r.SpendableAtoms != 0 || r.ImmatureAtoms != 0 || r.UnconfirmedAtoms != 0) {
		return ErrDailyWalletProtocol
	}
	return nil
}
func dailyWalletExpectedBinding(c RecoverableRemotePoolConfig, v RemotePoolView) (DailyWalletBinding, error) {
	id, e := RecoverableRemotePoolConfigID(c)
	if e != nil || !remoteWireViewValid(v, c.Base) {
		return DailyWalletBinding{}, ErrDailyWalletProtocol
	}
	pid, e := registry.IssueProfileID(c.IssueProfile)
	if e != nil {
		return DailyWalletBinding{}, ErrDailyWalletProtocol
	}
	dc, e := remotePoolDailyClientConfig(c.Base, "")
	if e != nil {
		return DailyWalletBinding{}, ErrDailyWalletProtocol
	}
	l := c.Base.ExpectedLocal
	return DailyWalletBinding{v, id, c.Base.Miners.SourceID, dc.SettlementConfigID, dc.RecipientScript, pid, l.OriginalSourceBindingID, l.WalletOwnerID, l.AllocationBindingID}, nil
}
func dailyWalletJobValid(j DailyWalletJob) bool {
	if j.Sequence == 0 || (j.CaptureSequence == 0) != (j.BeforeBatch == 0) || j.CaptureSequence >= j.Sequence || j.BeforeBatch > poolsettle.MaxDailyPayoutBatches+1 {
		return false
	}
	if j.State == "failed" {
		return stableOne(j.Error, "busy", "rate_limited", "stale_capture", "unauthorized", "unavailable", "cancelled")
	}
	return stableOne(j.State, "working", "done", "cancelled") && j.Error == ""
}
func dailyWalletCaptureValid(x *DailyWalletCapture, c RecoverableRemotePoolConfig, v RemotePoolView) bool {
	if x == nil {
		return true
	}
	want, e := dailyWalletExpectedBinding(c, v)
	if e != nil || x.Sequence == 0 || x.Binding != want || x.CapturedAtMS <= 0 || x.CapturedAtMS > 253402300799999 {
		return false
	}
	for _, p := range []poolsettle.DailyPayoutPage{x.First, x.Current} {
		if _, e := poolsettle.ValidateDailyPayoutPage(p, v.GymID); e != nil || p.BindingID != want.SourceBindingID || p.SettlementConfigID != want.SettlementConfigID || p.RecipientScript != want.RecipientScript {
			return false
		}
	}
	a, b := x.First, x.Current
	if a.BeforeBatch != a.HighBatch+1 || a.Revision != b.Revision || a.HighBatch != b.HighBatch || a.Balance != b.Balance || a.ObservedAtMS != b.ObservedAtMS || b.BeforeBatch > a.BeforeBatch || (a.BeforeBatch-b.BeforeBatch)%poolsettle.MaxDailyPayoutSlots != 0 {
		return false
	}
	if a.BeforeBatch == b.BeforeBatch && !reflect.DeepEqual(a, b) {
		return false
	}
	return true
}
func dailyWalletReadValid(r DailyWalletRead, q DailyWalletRequest, c RecoverableRemotePoolConfig, v RemotePoolView) bool {
	if !dailyWalletJobValid(r.Job) || r.Job.Sequence != q.Sequence || !dailyWalletCaptureValid(r.Capture, c, v) {
		return false
	}
	j, x := r.Job, r.Capture
	if x != nil && (x.Sequence > j.Sequence || j.State != "done" && x.Sequence == j.Sequence) {
		return false
	}
	if x != nil && j.CaptureSequence != 0 && x.Sequence != j.CaptureSequence {
		return false
	}
	if j.State == "done" {
		if x == nil {
			return false
		}
		if j.CaptureSequence == 0 {
			return x.Sequence == j.Sequence && !x.Stale && x.First.BeforeBatch == x.Current.BeforeBatch
		}
		return x.Sequence == j.CaptureSequence && (x.First.BeforeBatch == j.BeforeBatch || x.Current.BeforeBatch == j.BeforeBatch)
	}
	return true
}
func dailyWalletErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "stale_view", "busy", "unauthorized", "forbidden", "cancelled", "unavailable", "stale_capture", "not_found", "rate_limited", "outcome_unknown", "cleanup_incomplete")
}
func ValidateDailyWalletResponse(r DailyWalletResponse, q DailyWalletRequest, c RecoverableRemotePoolConfig, prior *RemotePoolView) error {
	bad := ErrDailyWalletProtocol
	if ValidateDailyWalletRequest(q, c) != nil || !dailyWalletFits(r, MaxDailyWalletResponseSize) || r.Version != q.Version || r.Application != q.Application || r.ID != q.ID || r.Command != q.Command || r.ConfigID != q.ConfigID || r.ViewID != q.ViewID {
		return bad
	}
	if prior != nil && !remoteWireViewValid(*prior, c.Base) || (q.ViewID != "") != (prior != nil) || prior != nil && q.ViewID != prior.ViewID {
		return bad
	}
	if !r.OK {
		if r.Snapshot != nil || r.Job != nil || r.Read != nil || !dailyWalletErrorCode(r.Error) {
			return bad
		}
		return nil
	}
	if r.Error != "" {
		return bad
	}
	if dailyWalletMoneyCommand(q.Command) {
		if prior == nil || r.Snapshot != nil {
			return bad
		}
		if q.Command == "daily_payout_begin" {
			if r.Job == nil || r.Read != nil || !dailyWalletJobValid(*r.Job) || r.Job.Sequence != q.Sequence || r.Job.CaptureSequence != q.CaptureSequence || r.Job.BeforeBatch != q.BeforeBatch {
				return bad
			}
		} else if r.Job != nil || r.Read == nil || !dailyWalletReadValid(*r.Read, q, c, *prior) {
			return bad
		}
		return nil
	}
	if r.Job != nil || r.Read != nil || r.Snapshot == nil || ValidateDailyWalletSnapshot(*r.Snapshot, c) != nil {
		return bad
	}
	s := r.Snapshot
	switch q.Command {
	case "login_fixture":
		if s.State != "signed_in" || s.Remote.State != "stopped" {
			return bad
		}
	case "logout":
		if s.State != "signed_out" {
			return bad
		}
	case "start", "stop":
		if s.State != "signed_in" || prior == nil || *s.View != *prior {
			return bad
		}
		if q.Command == "stop" && (!stableOne(s.Remote.State, "stopped", "failed") || s.Remote.HashesPerSecondMilli != 0) {
			return bad
		}
	case "snapshot":
		if prior == nil {
			if s.State == "signed_in" || q.ID == 1 && s.State != "signed_out" {
				return bad
			}
		} else if s.State == "signed_in" && *s.View != *prior {
			return bad
		}
	}
	return nil
}
func ParseDailyWalletResponse(raw []byte, q DailyWalletRequest, c RecoverableRemotePoolConfig, prior *RemotePoolView) (DailyWalletResponse, error) {
	var r DailyWalletResponse
	if dailyWalletDecode(raw, MaxDailyWalletResponseSize, &r) != nil || ValidateDailyWalletResponse(r, q, c, prior) != nil {
		return DailyWalletResponse{}, ErrDailyWalletProtocol
	}
	return r, nil
}
func DailyWalletJobProjection(j RemoteDailyPayoutJob) DailyWalletJob {
	return DailyWalletJob{j.Sequence, j.CaptureSequence, j.BeforeBatch, j.State, j.Error}
}
func DailyWalletReadProjection(r RemoteDailyPayoutRead) DailyWalletRead {
	out := DailyWalletRead{Job: DailyWalletJobProjection(r.Job)}
	if x := r.Capture; x != nil {
		b := x.Binding
		out.Capture = &DailyWalletCapture{x.Sequence, DailyWalletBinding{b.View, b.ConfigID, b.SourceBindingID, b.SettlementConfigID, b.RecipientScript, b.IssueProfileID, b.OriginalSourceBindingID, b.WalletOwnerID, b.AllocationBindingID}, x.CapturedAtMS, x.Stale, cloneDailyPayoutPage(x.First), cloneDailyPayoutPage(x.Current)}
	}
	return out
}
