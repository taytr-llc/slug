// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
)

const MaxRemotePoolRequestSize = 2048
const MaxRemotePoolResponseSize = 16384
const remotePoolWireMaxSafe = uint64(9007199254740991)
const remotePoolWireMaxAtoms = int64(2100000000000000)

var ErrRemotePoolProtocol = errors.New("remote pool protocol unavailable")

type RemotePoolRequest struct {
	Version  uint32 `json:"version"`
	ID       uint64 `json:"id"`
	Command  string `json:"command"`
	ConfigID string `json:"config_id"`
	ViewID   string `json:"view_id"`
	Fixture  string `json:"fixture"`
}

// RemotePoolWireSnapshot preserves the real public controller projection.
// Explicit null fields belong only to this new wire, not the old DTO encoding.
type RemotePoolWireSnapshot struct {
	Version  uint32              `json:"version"`
	ConfigID string              `json:"config_id"`
	State    string              `json:"state"`
	Account  AccountSnapshot     `json:"account"`
	View     *RemotePoolView     `json:"view"`
	Remote   *RemotePoolSnapshot `json:"remote"`
	Error    string              `json:"error"`
}

type RemotePoolResponse struct {
	Version  uint32                  `json:"version"`
	ID       uint64                  `json:"id"`
	Command  string                  `json:"command"`
	ConfigID string                  `json:"config_id"`
	ViewID   string                  `json:"view_id"`
	OK       bool                    `json:"ok"`
	Result   *RemotePoolWireSnapshot `json:"result"`
	Error    string                  `json:"error"`
}

// The production helper supplies its retained concrete controller. Public wire
// objects are correlation only and cannot create an account or Mining lease.
type RemotePoolControllerAPI interface {
	WireSnapshot(context.Context) (RemotePoolWireSnapshot, error)
	LoginFixture(context.Context, string) error
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Logout(context.Context, string) error
	Close() error
}

func remoteWireASCII(s string) bool {
	for i := range s {
		if s[i] < 32 || s[i] > 126 {
			return false
		}
	}
	return true
}

// Typed callers are bounded before marshaling. These DTOs have no recursive
// pointers, maps, slices or arbitrary interface fields.
func remoteWireBound(v reflect.Value, depth int, left *int) bool {
	if !v.IsValid() || depth > 8 || *left < 0 {
		return false
	}
	switch v.Kind() {
	case reflect.Pointer:
		return v.IsNil() || remoteWireBound(v.Elem(), depth+1, left)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !remoteWireBound(v.Field(i), depth+1, left) {
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
		return v.Uint() <= remotePoolWireMaxSafe
	case reflect.Int64:
		return v.Int() >= 0 && uint64(v.Int()) <= remotePoolWireMaxSafe
	case reflect.Bool:
	default:
		return false
	}
	return true
}

func remoteWireFits(value any, limit int) bool {
	left := limit
	if !remoteWireBound(reflect.ValueOf(value), 0, &left) {
		return false
	}
	b, e := json.Marshal(value)
	return e == nil && len(b) <= limit
}

// The new wire permits harmless JSON whitespace/escapes but retains exact
// ordered fields. The existing owned codec remains byte-canonical and unchanged.
// This private decoder accepts only these two concrete, closed frame types.
func remoteWireDecode(raw []byte, limit int, out any) error {
	if len(raw) == 0 || len(raw) > limit {
		return ErrRemotePoolProtocol
	}
	for _, b := range raw {
		if b > 127 || b == '\r' || b == '\n' {
			return ErrRemotePoolProtocol
		}
	}
	t := reflect.TypeOf(out)
	if t != reflect.TypeOf((*RemotePoolRequest)(nil)) && t != reflect.TypeOf((*RemotePoolResponse)(nil)) {
		return ErrRemotePoolProtocol
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(reflect.Type, int) error
	visit = func(t reflect.Type, depth int) error {
		if depth > 8 {
			return ErrRemotePoolProtocol
		}
		v, e := d.Token()
		if e != nil {
			return ErrRemotePoolProtocol
		}
		if t.Kind() == reflect.Pointer {
			if v == nil {
				return nil
			}
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Struct:
			if v != json.Delim('{') {
				return ErrRemotePoolProtocol
			}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				name := f.Tag.Get("json")
				if f.PkgPath != "" || name == "" || strings.Contains(name, ",") || !d.More() {
					return ErrRemotePoolProtocol
				}
				key, e := d.Token()
				if e != nil || key != name || visit(f.Type, depth+1) != nil {
					return ErrRemotePoolProtocol
				}
			}
			if d.More() {
				return ErrRemotePoolProtocol
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return ErrRemotePoolProtocol
			}
		case reflect.String:
			s, ok := v.(string)
			if !ok || len(s) > limit || !remoteWireASCII(s) {
				return ErrRemotePoolProtocol
			}
		case reflect.Bool:
			if _, ok := v.(bool); !ok {
				return ErrRemotePoolProtocol
			}
		case reflect.Uint32, reflect.Uint64, reflect.Int64:
			n, ok := v.(json.Number)
			if !ok || strings.ContainsAny(string(n), "-.eE+") {
				return ErrRemotePoolProtocol
			}
			x, e := strconv.ParseUint(string(n), 10, t.Bits())
			if e != nil || x > remotePoolWireMaxSafe {
				return ErrRemotePoolProtocol
			}
		default:
			return ErrRemotePoolProtocol
		}
		return nil
	}
	if visit(t.Elem(), 0) != nil {
		return ErrRemotePoolProtocol
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrRemotePoolProtocol
	}
	if json.Unmarshal(raw, out) != nil {
		return ErrRemotePoolProtocol
	}
	return nil
}

func ValidateRemotePoolRequest(r RemotePoolRequest, expected RemotePoolConfig) error {
	id, e := RemotePoolConfigID(expected)
	if e != nil || !remoteWireFits(r, MaxRemotePoolRequestSize) || r.Version != 1 || r.ID == 0 || r.ConfigID != id || r.ViewID != "" && !remotePoolID(r.ViewID) {
		return ErrRemotePoolProtocol
	}
	fixture := ""
	switch r.Command {
	case "snapshot":
	case "login_fixture":
		fixture = expected.ExpectedLocal.Recipient.Config.Fixture
		if r.ViewID != "" {
			return ErrRemotePoolProtocol
		}
	case "start", "stop", "logout":
		if !remotePoolID(r.ViewID) {
			return ErrRemotePoolProtocol
		}
	default:
		return ErrRemotePoolProtocol
	}
	if r.Fixture != fixture {
		return ErrRemotePoolProtocol
	}
	return nil
}

func MarshalRemotePoolRequest(r RemotePoolRequest, expected RemotePoolConfig) ([]byte, error) {
	if e := ValidateRemotePoolRequest(r, expected); e != nil {
		return nil, e
	}
	return json.Marshal(r)
}

func ParseRemotePoolRequest(raw []byte, expected RemotePoolConfig) (RemotePoolRequest, error) {
	var r RemotePoolRequest
	if remoteWireDecode(raw, MaxRemotePoolRequestSize, &r) != nil || ValidateRemotePoolRequest(r, expected) != nil {
		return RemotePoolRequest{}, ErrRemotePoolProtocol
	}
	return r, nil
}

func remoteWireViewValid(v RemotePoolView, c RemotePoolConfig) bool {
	r := c.ExpectedLocal.Recipient.Config
	return v.Version == 1 && remotePoolID(v.ViewID) && v.Generation > 0 && v.Generation <= remotePoolWireMaxSafe &&
		v.AccountAuthorityID == r.AccountAuthorityID && v.AccountID == r.AccountID && v.Fixture == r.Fixture &&
		v.RootPublicKey == r.Identity.RootPublicKey && v.GymPublicKey == r.Identity.GymPublicKey && v.GymID == r.Identity.GymID
}

func ValidateRemotePoolWireSnapshot(s RemotePoolWireSnapshot, expected RemotePoolConfig) error {
	id, e := RemotePoolConfigID(expected)
	if e != nil || !remoteWireFits(s, MaxRemotePoolResponseSize) || s.Version != 1 || s.ConfigID != id {
		return ErrRemotePoolProtocol
	}
	a := s.Account
	if !stableOne(s.State, "signed_out", "signed_in", "failed", "expired") || a.State != s.State || a.Fixture != expected.ExpectedLocal.Recipient.Config.Fixture || a.CanExport || s.Error != a.LastError {
		return ErrRemotePoolProtocol
	}
	if s.State != "signed_in" {
		if s.View != nil || s.Remote != nil || a.ID != "" || a.Generation != 0 || a.HoldUntilMS != 0 || a.Held {
			return ErrRemotePoolProtocol
		}
		if s.State == "signed_out" && s.Error != "" || s.State == "failed" && s.Error != "remote_unavailable" || s.State == "expired" && !stableOne(s.Error, "", "remote_unavailable") {
			return ErrRemotePoolProtocol
		}
		return nil
	}
	if s.Error != "" || s.View == nil || s.Remote == nil || !remoteWireViewValid(*s.View, expected) ||
		a.ID != s.View.AccountID || a.Generation != s.View.Generation || a.Held && a.HoldUntilMS == 0 {
		return ErrRemotePoolProtocol
	}
	r := s.Remote
	if r.Version != 1 || r.ConfigID != id || r.View != *s.View || r.SourceBindingID != expected.Miners.SourceID ||
		r.OriginalSourceBindingID != expected.ExpectedLocal.OriginalSourceBindingID || r.PayoutAddress != expected.ExpectedLocal.Recipient.Recipient.Address ||
		!stableOne(r.State, "stopped", "starting", "running", "failed") || remotePoolWorkerState(r.WorkerState) != r.WorkerState || r.WorkerState == "" || remotePoolPublicCode(r.LastError) != r.LastError ||
		r.State != "running" && r.HashesPerSecondMilli != 0 || r.Records > expected.MaxRecords || r.Pending > r.Records ||
		!stableOne(r.WalletState, "unavailable", "syncing", "ready") || r.WalletHeight > 2147483647 ||
		r.SpendableAtoms > remotePoolWireMaxAtoms || r.ImmatureAtoms > remotePoolWireMaxAtoms || r.UnconfirmedAtoms > remotePoolWireMaxAtoms {
		return ErrRemotePoolProtocol
	}
	if r.WalletState == "unavailable" && (r.WalletHeight != 0 || r.SpendableAtoms != 0 || r.ImmatureAtoms != 0 || r.UnconfirmedAtoms != 0) {
		return ErrRemotePoolProtocol
	}
	return nil
}

func remoteWireErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "stale_view", "busy", "unauthorized", "forbidden", "cancelled", "remote_unavailable", "outcome_unknown", "cleanup_incomplete")
}

func ParseRemotePoolResponse(raw []byte, q RemotePoolRequest, expected RemotePoolConfig, prior *RemotePoolView) (RemotePoolResponse, error) {
	var r RemotePoolResponse
	bad := func() (RemotePoolResponse, error) { return RemotePoolResponse{}, ErrRemotePoolProtocol }
	if ValidateRemotePoolRequest(q, expected) != nil || remoteWireDecode(raw, MaxRemotePoolResponseSize, &r) != nil ||
		r.Version != q.Version || r.ID != q.ID || r.Command != q.Command || r.ConfigID != q.ConfigID || r.ViewID != q.ViewID {
		return bad()
	}
	if prior != nil && !remoteWireViewValid(*prior, expected) {
		return bad()
	}
	if q.ViewID != "" {
		if prior == nil || prior.ViewID != q.ViewID {
			return bad()
		}
	} else if prior != nil {
		return bad()
	}
	if !r.OK {
		if r.Result != nil || !remoteWireErrorCode(r.Error) {
			return bad()
		}
		return r, nil
	}
	if r.Error != "" || r.Result == nil || ValidateRemotePoolWireSnapshot(*r.Result, expected) != nil {
		return bad()
	}
	s := r.Result
	switch q.Command {
	case "login_fixture":
		if s.State != "signed_in" || s.Remote.State != "stopped" {
			return bad()
		}
	case "logout":
		if s.State != "signed_out" {
			return bad()
		}
	case "start", "stop":
		if s.State != "signed_in" || prior == nil || *s.View != *prior {
			return bad()
		}
		if q.Command == "stop" && (!stableOne(s.Remote.State, "stopped", "failed") || s.Remote.HashesPerSecondMilli != 0) {
			return bad()
		}
	case "snapshot":
		if prior == nil {
			if s.State == "signed_in" || q.ID == 1 && s.State != "signed_out" {
				return bad()
			}
		} else if s.State == "signed_in" && *s.View != *prior {
			return bad()
		}
	}
	return r, nil
}
