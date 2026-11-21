// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

const RecoverableRemotePoolMaxConfigBytes = RemotePoolMaxConfigBytes + 1024
const remotePoolIssueApplication = "slug-remote-pool-issue-recovery"

type RecoverableRemotePoolConfig struct {
	Version      uint32                `json:"version"`
	Base         RemotePoolConfig      `json:"base"`
	IssueProfile registry.IssueProfile `json:"issue_profile"`
}
type RecoverableRemotePoolInputs struct {
	AttachmentRoot          string
	Config                  RecoverableRemotePoolConfig
	NodeRPC                 prospectnode.RPCConfig
	PoolCredentialPath      string
	AdmissionCredentialPath string
}

func (in RecoverableRemotePoolInputs) BaseInputs() RemotePoolInputs {
	return RemotePoolInputs{in.AttachmentRoot, in.Config.Base, in.NodeRPC, in.PoolCredentialPath, in.AdmissionCredentialPath}
}
func ValidateRecoverableRemotePoolConfig(c RecoverableRemotePoolConfig) error {
	if c.Version != 1 || ValidateDailyRemotePoolConfig(c.Base) != nil {
		return ErrRemotePoolConfig
	}
	_, e := remotePoolRecoverableWorkerConfig(c)
	return e
}
func MarshalRecoverableRemotePoolConfig(c RecoverableRemotePoolConfig) ([]byte, error) {
	if ValidateRecoverableRemotePoolConfig(c) != nil {
		return nil, ErrRemotePoolConfig
	}
	raw, e := json.Marshal(c)
	if e != nil || len(raw) > RecoverableRemotePoolMaxConfigBytes {
		return nil, ErrRemotePoolConfig
	}
	return raw, nil
}
func ParseRecoverableRemotePoolConfig(raw []byte) (RecoverableRemotePoolConfig, error) {
	var c RecoverableRemotePoolConfig
	if ownedcodec.Decode(raw, RecoverableRemotePoolMaxConfigBytes, &c) != nil || ValidateRecoverableRemotePoolConfig(c) != nil {
		return RecoverableRemotePoolConfig{}, ErrRemotePoolConfig
	}
	return c, nil
}
func remotePoolIssueDigest(label string, raw []byte) string {
	h := sha256.New()
	h.Write([]byte(label + "\x00v1\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
func RecoverableRemotePoolConfigID(c RecoverableRemotePoolConfig) (string, error) {
	raw, e := MarshalRecoverableRemotePoolConfig(c)
	if e != nil {
		return "", e
	}
	return remotePoolIssueDigest("SLUG pool issue recovery attachment", raw), nil
}
func remotePoolRecoverableWorkerConfig(c RecoverableRemotePoolConfig) (prospectworker.RecoverablePoolConfig, error) {
	base, e := remotePoolWorkerConfig(c.Base)
	if e != nil {
		return prospectworker.RecoverablePoolConfig{}, e
	}
	dc, e := remotePoolDailyClientConfig(c.Base, "")
	if e != nil {
		return prospectworker.RecoverablePoolConfig{}, e
	}
	pid, e := registry.IssueProfileID(c.IssueProfile)
	if e != nil {
		return prospectworker.RecoverablePoolConfig{}, ErrRemotePoolConfig
	}
	out := prospectworker.RecoverablePoolConfig{Version: 1, Base: base, IssueProfile: c.IssueProfile, SettlementConfigID: dc.SettlementConfigID, RecipientScript: dc.RecipientScript, TransportTrustID: remotePoolIssueDigest("SLUG pool issue recovery transport", []byte(base.TransportTrustID+pid))}
	if _, e = prospectworker.ValidateRecoverableDailyPoolConfig(out); e != nil {
		return prospectworker.RecoverablePoolConfig{}, ErrRemotePoolConfig
	}
	return out, nil
}
func remotePoolRecoverableMarkerBytes(c RecoverableRemotePoolConfig) ([]byte, []byte, error) {
	id, e := RecoverableRemotePoolConfigID(c)
	if e != nil {
		return nil, nil, e
	}
	intent, e := json.Marshal(struct {
		Version     uint32                      `json:"version"`
		Application string                      `json:"application"`
		ConfigID    string                      `json:"config_id"`
		Config      RecoverableRemotePoolConfig `json:"config"`
	}{1, remotePoolIssueApplication, id, c})
	if e != nil {
		return nil, nil, e
	}
	intent = append(intent, '\n')
	sum := sha256.Sum256(intent)
	ready, e := json.Marshal(remotePoolReady{1, remotePoolIssueApplication, id, hex.EncodeToString(sum[:])})
	return intent, append(ready, '\n'), e
}

type remotePoolRecoverableCredential struct {
	Version        uint32                     `json:"version"`
	Base           remotePoolCredentialRecord `json:"base"`
	IssueProfileID string                     `json:"issue_profile_id"`
}

func (o *remotePoolCredentials) parseCredential(raw []byte, audience string) (remotePoolCredentialRecord, error) {
	if o.recovery == nil {
		return remotePoolParseSelectedCredential(raw, o.configID, o.config.ExpectedLocal.Recipient.Config.Identity.GymID, audience, o.config.Version)
	}
	var w remotePoolRecoverableCredential
	if len(raw) < 2 || len(raw) > remotePoolCredentialLimit+1024 || raw[len(raw)-1] != '\n' || ownedcodec.Decode(raw[:len(raw)-1], remotePoolCredentialLimit+1023, &w) != nil || w.Version != 1 {
		return remotePoolCredentialRecord{}, errRemotePoolCredential
	}
	id, e := registry.IssueProfileID(o.recovery.IssueProfile)
	if e != nil || w.IssueProfileID != id {
		return remotePoolCredentialRecord{}, errRemotePoolCredential
	}
	base, e := json.Marshal(w.Base)
	if e != nil {
		return remotePoolCredentialRecord{}, errRemotePoolCredential
	}
	return remotePoolParseSelectedCredential(append(base, '\n'), o.configID, o.config.ExpectedLocal.Recipient.Config.Identity.GymID, audience, 2)
}
func OpenRecoverableDailyRemotePoolAttachment(ctx context.Context, s *OwnedWalletSession, view string, in RecoverableRemotePoolInputs) (*RemotePoolAttachment, error) {
	if wire.TestPoW {
		return nil, ErrRemotePoolProfile
	}
	raw, e := MarshalRecoverableRemotePoolConfig(in.Config)
	if e != nil {
		return nil, e
	}
	c, e := ParseRecoverableRemotePoolConfig(raw)
	if e != nil {
		return nil, e
	}
	in.Config = c
	return openRemotePoolAttachmentSelection(ctx, s, view, in.BaseInputs(), true, &c)
}
func (p *remotePoolAuthority) ResumeIssue(ctx context.Context, id string) (poolservice.IssueResponse, error) {
	return remotePoolWorkerCall(ctx, p.owner, true, false, func(c context.Context) (poolservice.IssueResponse, error) { return p.lease.ResumeIssue(c, id) })
}
func (p *remotePoolAuthority) LookupIssue(ctx context.Context, id string) (poolservice.IssueResponse, error) {
	return remotePoolWorkerCall(ctx, p.owner, false, false, func(c context.Context) (poolservice.IssueResponse, error) { return p.lease.LookupIssue(c, id) })
}
func (l *remotePoolPoolLease) ResumeIssue(ctx context.Context, id string) (poolservice.IssueResponse, error) {
	return remotePoolLeaseCall(ctx, l.lease, true, func(c context.Context) (poolservice.IssueResponse, error) { return l.lease.pool.ResumeIssue(c, id) })
}
func (l *remotePoolPoolLease) LookupIssue(ctx context.Context, id string) (poolservice.IssueResponse, error) {
	return remotePoolLeaseCall(ctx, l.lease, false, func(c context.Context) (poolservice.IssueResponse, error) { return l.lease.pool.LookupIssue(c, id) })
}
func sameRecoverableRemoteBase(c RecoverableRemotePoolConfig, b RemotePoolConfig) bool {
	x, _ := MarshalDailyRemotePoolConfig(c.Base)
	y, _ := MarshalDailyRemotePoolConfig(b)
	return bytes.Equal(x, y)
}
