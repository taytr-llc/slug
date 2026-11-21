// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"path/filepath"

	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
)

func ValidateDailyRemotePoolConfig(c RemotePoolConfig) error {
	return validateRemotePoolConfig(c, true)
}
func MarshalDailyRemotePoolConfig(c RemotePoolConfig) ([]byte, error) {
	if ValidateDailyRemotePoolConfig(c) != nil {
		return nil, ErrRemotePoolConfig
	}
	return json.Marshal(c)
}
func ParseDailyRemotePoolConfig(raw []byte) (RemotePoolConfig, error) {
	var c RemotePoolConfig
	if ownedcodec.Decode(raw, RemotePoolMaxConfigBytes, &c) != nil || ValidateDailyRemotePoolConfig(c) != nil {
		return RemotePoolConfig{}, ErrRemotePoolConfig
	}
	return c, nil
}
func DailyRemotePoolConfigID(c RemotePoolConfig) (string, error) {
	raw, e := MarshalDailyRemotePoolConfig(c)
	if e != nil {
		return "", e
	}
	h := sha256.New()
	h.Write([]byte("SLUG remote pool attachment\x00v2\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func marshalSelectedRemotePoolConfig(c RemotePoolConfig, daily bool) ([]byte, error) {
	if daily {
		return MarshalDailyRemotePoolConfig(c)
	}
	return MarshalRemotePoolConfig(c)
}
func parseSelectedRemotePoolConfig(raw []byte, daily bool) (RemotePoolConfig, error) {
	if daily {
		return ParseDailyRemotePoolConfig(raw)
	}
	return ParseRemotePoolConfig(raw)
}
func selectedRemotePoolConfigID(c RemotePoolConfig) (string, error) {
	if c.Version == 2 {
		return DailyRemotePoolConfigID(c)
	}
	return RemotePoolConfigID(c)
}

// This private projection binds transport to the independently selected full
// settlement and the original local wallet recipient. It grants no wallet right.
func remotePoolDailyClientConfig(c RemotePoolConfig, token string) (poolservice.DailyClientConfig, error) {
	id, e := poolsettle.DailyConfigBindingID(c.Settlement)
	if e != nil || c.Version != 2 {
		return poolservice.DailyClientConfig{}, ErrRemotePoolConfig
	}
	return poolservice.DailyClientConfig{Version: 2, URL: c.Origin, Token: token, CACertificate: c.CACertificate, PublicKey: c.ExpectedLocal.Recipient.Config.Identity.GymPublicKey, Source: c.Source, SettlementConfigID: id, RecipientScript: c.ExpectedLocal.Recipient.Recipient.Script}, nil
}

func OpenDailyRemotePoolAttachment(ctx context.Context, session *OwnedWalletSession, viewID string, in RemotePoolInputs) (*RemotePoolAttachment, error) {
	return openRemotePoolAttachment(ctx, session, viewID, in, true)
}

// Daily work never repairs recipient controls. Recheck both original markers
// and wallet READY at operation/signature fences without a node call or a new
// allocation. The original View is checked again after these bounded reads.
func (l *remotePoolLocal) dailyRecipientControls(ctx context.Context) error {
	s := l.session
	want := ownedWalletReady{1, allocationJSONHash(s.intent), s.intent.BindingID, s.intent.CarrierBindingID, s.intent.WithdrawalBindingID, l.expected.Recipient}
	if e := readPoolRuntimeRecord(filepath.Join(s.root, ownedWalletReadyName), want); e != nil {
		return e
	}
	if e := auditOwnedWalletRecipientControls(s.root, &want); e != nil {
		return e
	}
	return l.check(ctx)
}
