// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/poolsettle"
)

const DailyWalletApplication2 = "SLUG-REMOTE-DAILY-WALLET-2"
const MaxDailyWalletResponseSize2 = MaxWalletActivityResponseSize

type DailyWalletRequest2 struct {
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
	After           uint32 `json:"after"`
	Limit           uint32 `json:"limit"`
	TxID            string `json:"txid"`
	OutputIndex     uint32 `json:"output_index"`
	BatchSequence   uint64 `json:"batch_sequence,string"`
}
type DailyWalletActivityJob struct {
	Sequence uint64 `json:"sequence,string"`
	State    string `json:"state"`
	Error    string `json:"error"`
}
type DailyWalletActivityCapture struct {
	Sequence uint64                `json:"sequence,string"`
	Binding  DailyWalletBinding    `json:"binding"`
	Metadata WalletActivityCapture `json:"metadata"`
}
type DailyWalletActivityRead struct {
	Job     DailyWalletActivityJob      `json:"job"`
	Capture *DailyWalletActivityCapture `json:"capture"`
}
type DailyWalletActivityPage struct {
	Capture DailyWalletActivityCapture `json:"capture"`
	After   uint32                     `json:"after"`
	Limit   uint32                     `json:"limit"`
	Next    uint32                     `json:"next"`
	HasMore bool                       `json:"has_more"`
	Items   []WalletActivityItem       `json:"items"`
}
type DailyWalletActivityDetail struct {
	Capture DailyWalletActivityCapture `json:"capture"`
	Item    WalletActivityItem         `json:"item"`
	Outputs []WalletActivityOutput     `json:"outputs"`
}
type DailyWalletPayoutObservation struct {
	CaptureSequence        uint64                `json:"capture_sequence,string"`
	Binding                DailyWalletBinding    `json:"binding"`
	Revision               uint64                `json:"revision,string"`
	HighBatch              uint64                `json:"high_batch,string"`
	CapturedAtMS           int64                 `json:"captured_at_ms"`
	SettlementObservedAtMS int64                 `json:"settlement_observed_at_ms"`
	Stale                  bool                  `json:"stale"`
	BatchSequence          uint64                `json:"batch_sequence,string"`
	Payment                poolsettle.PayoutItem `json:"payment"`
	ObservedAtMS           int64                 `json:"observed_at_ms"`
	State                  string                `json:"state"`
	Error                  string                `json:"error"`
	WalletSynced           bool                  `json:"wallet_synced"`
	WalletHeight           uint32                `json:"wallet_height"`
	WalletTip              string                `json:"wallet_tip"`
	Confirmations          uint32                `json:"confirmations"`
	BlockHash              string                `json:"block_hash"`
}
type DailyWalletResponse2 struct {
	Version           uint32                        `json:"version"`
	Application       string                        `json:"application"`
	ID                uint64                        `json:"id,string"`
	Command           string                        `json:"command"`
	ConfigID          string                        `json:"config_id"`
	ViewID            string                        `json:"view_id"`
	OK                bool                          `json:"ok"`
	Snapshot          *RemotePoolWireSnapshot       `json:"snapshot"`
	Job               *DailyWalletJob               `json:"job"`
	Read              *DailyWalletRead              `json:"read"`
	ActivityJob       *DailyWalletActivityJob       `json:"activity_job"`
	ActivityRead      *DailyWalletActivityRead      `json:"activity_read"`
	ActivityPage      *DailyWalletActivityPage      `json:"activity_page"`
	ActivityDetail    *DailyWalletActivityDetail    `json:"activity_detail"`
	PayoutObservation *DailyWalletPayoutObservation `json:"payout_observation"`
	Error             string                        `json:"error"`
}
type DailyWalletControllerAPI2 interface {
	DailyWalletControllerAPI
	BeginWalletActivityRead(context.Context, string, uint64) (RemoteWalletActivityJob, error)
	WalletActivityRead(context.Context, string, uint64) (RemoteWalletActivityRead, error)
	CancelWalletActivityRead(context.Context, string, uint64) error
	WalletActivityPage(context.Context, string, uint64, uint32, uint32) (RemoteWalletActivityPage, error)
	WalletActivityDetail(context.Context, string, uint64, string) (RemoteWalletActivityDetail, error)
	ObserveDailyPayout(context.Context, string, uint64, uint64, string, uint32) (RemoteDailyPayoutObservation, error)
}

func dailyWalletCommand2(command string) bool {
	return stableOne(command, "wallet_activity_begin", "wallet_activity_read", "wallet_activity_cancel", "wallet_activity_page", "wallet_activity_detail", "daily_payout_observe")
}
func DailyWalletResponseLimit2(command string) int {
	switch command {
	case "wallet_activity_page", "wallet_activity_detail":
		return MaxWalletActivityResponseSize
	case "wallet_activity_begin", "wallet_activity_read", "wallet_activity_cancel", "daily_payout_observe":
		return MaxWalletActivitySmallResponseSize
	}
	return MaxDailyWalletResponseSize
}
func dailyWalletRequest1(q DailyWalletRequest2) DailyWalletRequest {
	return DailyWalletRequest{Version: 1, Application: DailyWalletApplication, ID: q.ID, Command: q.Command, ConfigID: q.ConfigID, ViewID: q.ViewID, Fixture: q.Fixture, Sequence: q.Sequence, CaptureSequence: q.CaptureSequence, BeforeBatch: q.BeforeBatch}
}
func dailyWalletResponse1(r DailyWalletResponse2) DailyWalletResponse {
	return DailyWalletResponse{Version: 1, Application: DailyWalletApplication, ID: r.ID, Command: r.Command, ConfigID: r.ConfigID, ViewID: r.ViewID, OK: r.OK, Snapshot: r.Snapshot, Job: r.Job, Read: r.Read, Error: r.Error}
}
func ValidateDailyWalletRequest2(q DailyWalletRequest2, c RecoverableRemotePoolConfig) error {
	id, e := RecoverableRemotePoolConfigID(c)
	if e != nil || q.Version != 2 || q.Application != DailyWalletApplication2 || q.ID == 0 || q.ConfigID != id || !dailyWalletFits2(q, MaxDailyWalletRequestSize) {
		return ErrDailyWalletProtocol
	}
	if !dailyWalletCommand2(q.Command) {
		if q.After != 0 || q.Limit != 0 || q.TxID != "" || q.OutputIndex != 0 || q.BatchSequence != 0 {
			return ErrDailyWalletProtocol
		}
		return ValidateDailyWalletRequest(dailyWalletRequest1(q), c)
	}
	if q.ID == 1 || !remotePoolID(q.ViewID) || q.Fixture != "" || q.BeforeBatch != 0 {
		return ErrDailyWalletProtocol
	}
	switch q.Command {
	case "wallet_activity_begin", "wallet_activity_read", "wallet_activity_cancel":
		if q.Sequence == 0 || q.CaptureSequence != 0 || q.After != 0 || q.Limit != 0 || q.TxID != "" || q.OutputIndex != 0 || q.BatchSequence != 0 {
			return ErrDailyWalletProtocol
		}
	case "wallet_activity_page":
		if q.Sequence != 0 || q.CaptureSequence == 0 || q.After > 256 || q.Limit < 1 || q.Limit > 8 || q.TxID != "" || q.OutputIndex != 0 || q.BatchSequence != 0 {
			return ErrDailyWalletProtocol
		}
	case "wallet_activity_detail":
		if q.Sequence != 0 || q.CaptureSequence == 0 || q.After != 0 || q.Limit != 0 || !payoutLowerHex(q.TxID, 64, false) || q.OutputIndex != 0 || q.BatchSequence != 0 {
			return ErrDailyWalletProtocol
		}
	case "daily_payout_observe":
		if q.Sequence != 0 || q.CaptureSequence == 0 || q.After != 0 || q.Limit != 0 || !payoutLowerHex(q.TxID, 64, false) || q.OutputIndex > 255 || q.BatchSequence == 0 || q.BatchSequence > poolsettle.MaxDailyPayoutBatches {
			return ErrDailyWalletProtocol
		}
	}
	return nil
}

// This finite pre-marshal walk admits signed safe NetAtoms and <=256 outputs;
// semantic validators below independently restrict each field/container.
func dailyWalletBound2(v reflect.Value, depth int, quoted bool, left *int) bool {
	if !v.IsValid() || depth > 16 || *left < 0 {
		return false
	}
	switch v.Kind() {
	case reflect.Pointer:
		return v.IsNil() || dailyWalletBound2(v.Elem(), depth+1, quoted, left)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if f.PkgPath != "" || !dailyWalletBound2(v.Field(i), depth+1, strings.Contains(f.Tag.Get("json"), ",string"), left) {
				return false
			}
		}
	case reflect.Slice:
		if v.IsNil() || v.Len() > 256 {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !dailyWalletBound2(v.Index(i), depth+1, false, left) {
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
		if quoted {
			return v.Int() >= 0
		}
		return v.Int() >= -int64(remotePoolWireMaxSafe) && v.Int() <= int64(remotePoolWireMaxSafe)
	case reflect.Bool:
	default:
		return false
	}
	return true
}
func dailyWalletFits2(v any, limit int) bool {
	left := limit
	if !dailyWalletBound2(reflect.ValueOf(v), 0, false, &left) {
		return false
	}
	b, e := json.Marshal(v)
	return e == nil && len(b)+1 <= limit
}
func dailyWalletDecode2(raw []byte, limit int, out any) error {
	if len(raw) == 0 || len(raw)+1 > limit {
		return ErrDailyWalletProtocol
	}
	for _, b := range raw {
		if b < 32 || b > 126 {
			return ErrDailyWalletProtocol
		}
	}
	switch out.(type) {
	case *DailyWalletRequest2, *DailyWalletResponse2:
	default:
		return ErrDailyWalletProtocol
	}
	if json.Unmarshal(raw, out) != nil || !dailyWalletFits2(reflect.ValueOf(out).Elem().Interface(), limit) {
		return ErrDailyWalletProtocol
	}
	b, e := json.Marshal(out)
	if e != nil || !bytes.Equal(raw, b) {
		return ErrDailyWalletProtocol
	}
	return nil
}
func MarshalDailyWalletRequest2(q DailyWalletRequest2, c RecoverableRemotePoolConfig) ([]byte, error) {
	if ValidateDailyWalletRequest2(q, c) != nil {
		return nil, ErrDailyWalletProtocol
	}
	return json.Marshal(q)
}
func ParseDailyWalletRequest2(raw []byte, c RecoverableRemotePoolConfig) (DailyWalletRequest2, error) {
	var q DailyWalletRequest2
	if dailyWalletDecode2(raw, MaxDailyWalletRequestSize, &q) != nil || ValidateDailyWalletRequest2(q, c) != nil {
		return DailyWalletRequest2{}, ErrDailyWalletProtocol
	}
	return q, nil
}
func dailyWalletActivityJobValid(j DailyWalletActivityJob) bool {
	if j.Sequence == 0 {
		return false
	}
	if j.State == "failed" {
		return stableOne(j.Error, "busy", "capacity", "activity_unavailable")
	}
	return stableOne(j.State, "working", "done", "cancelled") && j.Error == ""
}
func dailyWalletActivityCaptureValid(x DailyWalletActivityCapture, c RecoverableRemotePoolConfig, v RemotePoolView) bool {
	want, e := dailyWalletExpectedBinding(c, v)
	return e == nil && x.Sequence != 0 && x.Binding == want && ValidateWalletActivityCapture(x.Metadata) == nil
}
func dailyWalletActivityReadValid(r DailyWalletActivityRead, q DailyWalletRequest2, c RecoverableRemotePoolConfig, v RemotePoolView) bool {
	if !dailyWalletActivityJobValid(r.Job) || r.Job.Sequence != q.Sequence {
		return false
	}
	x := r.Capture
	if x != nil && !dailyWalletActivityCaptureValid(*x, c, v) {
		return false
	}
	if r.Job.State == "done" {
		return x != nil && x.Sequence == r.Job.Sequence
	}
	return x == nil || x.Sequence < r.Job.Sequence
}
func dailyWalletActivityPageValid(p DailyWalletActivityPage, q DailyWalletRequest2, c RecoverableRemotePoolConfig, v RemotePoolView) bool {
	if !dailyWalletActivityCaptureValid(p.Capture, c, v) || p.Capture.Sequence != q.CaptureSequence || p.After != q.After || p.Limit != q.Limit || p.After > p.Capture.Metadata.Count || p.Items == nil {
		return false
	}
	n := min(p.Limit, p.Capture.Metadata.Count-p.After)
	if uint32(len(p.Items)) != n || p.Next != p.After+n || p.HasMore != (p.Next < p.Capture.Metadata.Count) {
		return false
	}
	seen := make(map[string]bool, len(p.Items))
	for i, item := range p.Items {
		if ValidateWalletActivityItem(item, p.Capture.Metadata) != nil || seen[item.TxID] || i > 0 && !WalletActivityItemLess(p.Items[i-1], item) {
			return false
		}
		seen[item.TxID] = true
	}
	return true
}
func dailyWalletActivityDetailValid(d DailyWalletActivityDetail, q DailyWalletRequest2, c RecoverableRemotePoolConfig, v RemotePoolView) bool {
	if !dailyWalletActivityCaptureValid(d.Capture, c, v) || d.Capture.Sequence != q.CaptureSequence || d.Capture.Metadata.Count == 0 || d.Item.TxID != q.TxID || ValidateWalletActivityItem(d.Item, d.Capture.Metadata) != nil || d.Outputs == nil || uint32(len(d.Outputs)) != d.Item.OutputCount {
		return false
	}
	var total, credit, change int64
	var owned uint32
	for i, o := range d.Outputs {
		if o.Index != uint32(i) || !activityAtoms(o.Atoms) || o.Atoms > poolsettle.MaxMoney-total || o.Change && !o.Owned || !activityAddress(o.Address) {
			return false
		}
		total += o.Atoms
		if o.Owned {
			owned++
			credit += o.Atoms
		}
		if o.Change {
			change += o.Atoms
		}
	}
	return owned == d.Item.OwnedOutputCount && credit == d.Item.ReceivedAtoms && change == d.Item.ChangeAtoms && (d.Item.FeeAtoms == nil || total+*d.Item.FeeAtoms == d.Item.SpentAtoms)
}
func dailyWalletObservationValid(o DailyWalletPayoutObservation, q DailyWalletRequest2, c RecoverableRemotePoolConfig, v RemotePoolView) bool {
	want, e := dailyWalletExpectedBinding(c, v)
	if e != nil || o.Binding != want || o.CaptureSequence != q.CaptureSequence || o.Revision == 0 || o.HighBatch == 0 || o.HighBatch > poolsettle.MaxDailyPayoutBatches || o.BatchSequence != q.BatchSequence || o.BatchSequence > o.HighBatch || !payoutTime(o.CapturedAtMS, false) || !payoutTime(o.SettlementObservedAtMS, false) || !payoutItemValid(o.Payment, o.SettlementObservedAtMS) || o.Payment.TxID != q.TxID || o.Payment.OutputIndex != q.OutputIndex || !payoutTime(o.ObservedAtMS, false) || o.WalletHeight > 2147483647 {
		return false
	}
	if o.State == "unavailable" {
		return o.Error == "wallet_unavailable" && !o.WalletSynced && o.WalletHeight == 0 && o.WalletTip == "" && o.Confirmations == 0 && o.BlockHash == ""
	}
	if o.Error != "" || !o.WalletSynced || !payoutNonzeroID(o.WalletTip) {
		return false
	}
	switch o.State {
	case "not_observed", "unconfirmed":
		return o.Confirmations == 0 && o.BlockHash == ""
	case "confirmed":
		return o.Confirmations > 0 && o.Confirmations <= o.WalletHeight+1 && payoutNonzeroID(o.BlockHash)
	}
	return false
}
func ValidateDailyWalletResponse2(r DailyWalletResponse2, q DailyWalletRequest2, c RecoverableRemotePoolConfig, prior *RemotePoolView) error {
	bad := ErrDailyWalletProtocol
	if ValidateDailyWalletRequest2(q, c) != nil || !dailyWalletFits2(r, DailyWalletResponseLimit2(q.Command)) || r.Version != q.Version || r.Application != q.Application || r.ID != q.ID || r.Command != q.Command || r.ConfigID != q.ConfigID || r.ViewID != q.ViewID {
		return bad
	}
	n := 0
	for _, yes := range []bool{r.ActivityJob != nil, r.ActivityRead != nil, r.ActivityPage != nil, r.ActivityDetail != nil, r.PayoutObservation != nil} {
		if yes {
			n++
		}
	}
	if !dailyWalletCommand2(q.Command) {
		if n != 0 {
			return bad
		}
		return ValidateDailyWalletResponse(dailyWalletResponse1(r), dailyWalletRequest1(q), c, prior)
	}
	if prior == nil || !remoteWireViewValid(*prior, c.Base) || q.ViewID != prior.ViewID || r.Snapshot != nil || r.Job != nil || r.Read != nil {
		return bad
	}
	if !r.OK {
		if n != 0 || !dailyWalletErrorCode(r.Error) {
			return bad
		}
		return nil
	}
	if r.Error != "" || n != 1 {
		return bad
	}
	valid := false
	switch q.Command {
	case "wallet_activity_begin":
		valid = r.ActivityJob != nil && dailyWalletActivityJobValid(*r.ActivityJob) && r.ActivityJob.Sequence == q.Sequence
	case "wallet_activity_read", "wallet_activity_cancel":
		valid = r.ActivityRead != nil && dailyWalletActivityReadValid(*r.ActivityRead, q, c, *prior)
	case "wallet_activity_page":
		valid = r.ActivityPage != nil && dailyWalletActivityPageValid(*r.ActivityPage, q, c, *prior)
	case "wallet_activity_detail":
		valid = r.ActivityDetail != nil && dailyWalletActivityDetailValid(*r.ActivityDetail, q, c, *prior)
	case "daily_payout_observe":
		valid = r.PayoutObservation != nil && dailyWalletObservationValid(*r.PayoutObservation, q, c, *prior)
	}
	if !valid {
		return bad
	}
	return nil
}
func ParseDailyWalletResponse2(raw []byte, q DailyWalletRequest2, c RecoverableRemotePoolConfig, prior *RemotePoolView) (DailyWalletResponse2, error) {
	var r DailyWalletResponse2
	if dailyWalletDecode2(raw, DailyWalletResponseLimit2(q.Command), &r) != nil || ValidateDailyWalletResponse2(r, q, c, prior) != nil {
		return DailyWalletResponse2{}, ErrDailyWalletProtocol
	}
	return r, nil
}
func dailyWalletBindingProjection(b RemoteDailyPayoutBinding) DailyWalletBinding {
	return DailyWalletBinding{b.View, b.ConfigID, b.SourceBindingID, b.SettlementConfigID, b.RecipientScript, b.IssueProfileID, b.OriginalSourceBindingID, b.WalletOwnerID, b.AllocationBindingID}
}
func DailyWalletActivityJobProjection(j RemoteWalletActivityJob) DailyWalletActivityJob {
	return DailyWalletActivityJob{j.Sequence, j.State, j.Error}
}
func DailyWalletActivityCaptureProjection(x RemoteWalletActivityCapture) DailyWalletActivityCapture {
	return DailyWalletActivityCapture{x.Sequence, dailyWalletBindingProjection(x.Binding), x.Metadata}
}
func DailyWalletActivityReadProjection(r RemoteWalletActivityRead) DailyWalletActivityRead {
	out := DailyWalletActivityRead{Job: DailyWalletActivityJobProjection(r.Job)}
	if r.Capture != nil {
		x := DailyWalletActivityCaptureProjection(*r.Capture)
		out.Capture = &x
	}
	return out
}
func DailyWalletActivityPageProjection(p RemoteWalletActivityPage) DailyWalletActivityPage {
	return DailyWalletActivityPage{DailyWalletActivityCaptureProjection(p.Capture), p.After, p.Limit, p.Next, p.HasMore, p.Items}
}
func DailyWalletActivityDetailProjection(d RemoteWalletActivityDetail) DailyWalletActivityDetail {
	return DailyWalletActivityDetail{DailyWalletActivityCaptureProjection(d.Capture), d.Item, d.Outputs}
}
func DailyWalletObservationProjection(o RemoteDailyPayoutObservation) DailyWalletPayoutObservation {
	return DailyWalletPayoutObservation{o.CaptureSequence, dailyWalletBindingProjection(o.Binding), o.Revision, o.HighBatch, o.CapturedAtMS, o.SettlementObservedAtMS, o.Stale, o.BatchSequence, o.Payment, o.ObservedAtMS, o.State, o.Error, o.WalletSynced, o.WalletHeight, o.WalletTip, o.Confirmations, o.BlockHash}
}
