// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
)

const manifestSafeInteger = uint64(1<<53 - 1)
const manifestMaxDate = int64(8640000000000000)

func manifestASCII(s string, min, max int) bool {
	if len(s) < min || len(s) > max {
		return false
	}
	for i := range len(s) {
		if s[i] < 32 || s[i] > 126 {
			return false
		}
	}
	return true
}
func validManifestView(v pilot.ManifestViewPin) bool {
	if v.Version != 1 || !publicHex.MatchString(v.ViewID) || !publicHex.MatchString(v.AccountID) || !publicHex.MatchString(v.AccountAuthorityID) || v.Generation == 0 || v.Generation > manifestSafeInteger || v.AuthorityPin.Version != 5 || v.AuthorityPin.Network != uint32(chaincfg.MainNetParams.Net) || v.AuthorityPin.Genesis != chaincfg.MainNetParams.GenesisHash.String() || v.AuthorityPin.AuthorityID == v.AccountAuthorityID || !publicHex.MatchString(v.AuthorityPin.AuthorityID) || !publicHex.MatchString(v.TransportTrustID) {
		return false
	}
	if _, err := fighterdescriptor.ResolveCandidate(v.AuthorityPin.DescriptorID); err != nil {
		return false
	}
	u, err := url.Parse(v.AuthorityOrigin)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || len(v.AuthorityOrigin) > 256 {
		return false
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || host != "127.0.0.1" {
		return false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port || v.AuthorityOrigin != "http://127.0.0.1:"+port {
		return false
	}
	trust, err := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: v.AuthorityOrigin, Token: strings.Repeat("0", 64), AuthorityPin: v.AuthorityPin})
	return err == nil && trust == v.TransportTrustID
}

// These transport checks are structural and cheap. Full work/static semantics
// are verified by the private projector before its immutable cache is published;
// polling does not regenerate manifests, render portraits or hash Argon.
func validManifestSnapshot(s *pilot.ManifestSnapshot) bool {
	if s.Version != 1 || s.Mode != pilot.ManifestMode || !oneOf(s.State, "stopped", "starting", "running", "stopping", "failed") || !oneOf(s.WalletState, "closed", "connecting", "syncing", "ready", "failed") || !oneOf(s.BackupState, "idle", "running", "succeeded", "failed", "cancelled") || !oneOf(s.BackupPhase, "", "opening", "checking", "restoring", "syncing", "verifying", "complete") || !oneOf(s.BackupError, "", "backup_unavailable", "backup_invalid", "backup_mismatch", "backup_storage_failed") {
		return false
	}
	if s.Attempts > manifestSafeInteger || s.Height > manifestSafeInteger || s.WalletHeight > manifestSafeInteger || s.Pending > 1000 || s.AcceptedCount > 1000 || s.SpendableAtoms < 0 || uint64(s.SpendableAtoms) > manifestSafeInteger || s.ImmatureAtoms < 0 || uint64(s.ImmatureAtoms) > manifestSafeInteger || math.IsNaN(s.HashesPerSecond) || math.IsInf(s.HashesPerSecond, 0) || s.HashesPerSecond < 0 || !manifestStatusCode(s.LastError) || !manifestStatusCode(s.RecoveryError) {
		return false
	}
	a := s.Account
	if !oneOf(a.State, "signed_out", "signing_in", "signed_in", "resetting", "signing_out", "failed", "expired") || !oneOf(a.Fixture, "", "zero256", "incrementing256") || !oneOf(a.LastError, "", "account_unavailable") || a.Generation > manifestSafeInteger || a.HoldUntilMS < 0 || a.HoldUntilMS > manifestMaxDate || a.Held && (a.CanExport || a.HoldUntilMS == 0) {
		return false
	}
	projection := s.Projection
	if projection.Items == nil || len(projection.Items) > pilot.MaxManifestProspects || !oneOf(projection.State, "idle", "loading", "ready", "failed", "cancelled") || !oneOf(projection.Error, "", "projection_unavailable", "projection_corrupt", "projection_cancelled") || projection.Revision > manifestSafeInteger || projection.ThroughSequence > s.AcceptedCount {
		return false
	}
	if a.State != "signed_in" {
		return s.View == nil && a.ID == "" && a.Generation == 0 && a.HoldUntilMS == 0 && !a.CanExport && !a.Held && s.GymID == "" && s.GymPublicKey == "" && s.PayoutAddress == "" && s.SpendableAtoms == 0 && s.ImmatureAtoms == 0 && s.AcceptedCount == 0 && s.Pending == 0 && s.Attempts == 0 && s.Height == 0 && s.WalletHeight == 0 && s.HashesPerSecond == 0 && len(projection.Items) == 0 && projection.ThroughSequence == 0 && projection.Revision == 0 && !projection.HasOlder && !projection.CompleteRecent
	}
	// Reuse the old fixture identity rule without accepting its old DTO shape.
	old := pilot.Snapshot{Account: &a, GymID: s.GymID, GymPublicKey: s.GymPublicKey}
	if !validAccountProjection(&old) || !payoutAddress.MatchString(s.PayoutAddress) || s.View == nil || !validManifestView(*s.View) || s.View.AccountID != a.ID || s.View.Generation != a.Generation {
		return false
	}
	if projection.State != "ready" && len(projection.Items) != 0 {
		return false
	}
	seen := make(map[string]bool, len(projection.Items))
	prev := uint64(1001)
	for _, item := range projection.Items {
		if !validManifestSummary(item) || item.Sequence >= prev || item.Sequence > s.AcceptedCount || item.DescriptorID != s.View.AuthorityPin.DescriptorID || seen[item.ID] {
			return false
		}
		seen[item.ID] = true
		prev = item.Sequence
	}
	through := uint64(0)
	if len(projection.Items) > 0 {
		through = projection.Items[0].Sequence
	}
	if projection.ThroughSequence != through || projection.HasOlder != (s.AcceptedCount > uint64(len(projection.Items))) {
		return false
	}
	complete := len(projection.Items) == int(min(s.AcceptedCount, 8))
	for i, item := range projection.Items {
		if item.Sequence != s.AcceptedCount-uint64(i) {
			complete = false
		}
	}
	// A not-yet-loaded empty page is not an affirmative verification claim.
	if projection.State != "ready" {
		complete = false
	}
	return projection.CompleteRecent == complete
}
func validManifestSummary(s pilot.ManifestProspectSummary) bool {
	if s.Sequence == 0 || s.Sequence > 1000 || !publicHex.MatchString(s.ID) || !publicHex.MatchString(s.WorkHash) || !publicHex.MatchString(s.DescriptorID) || !publicHex.MatchString(s.ManifestDigest) || !publicHex.MatchString(s.PortraitDigest) || s.BornAtMS <= 0 || s.BornAtMS > manifestMaxDate || s.AcceptedAtMS < s.BornAtMS || s.AcceptedAtMS >= s.BornAtMS+300000 || s.AcceptedAtMS > manifestMaxDate {
		return false
	}
	if !manifestASCII(s.Name.First, 1, 64) || !manifestASCII(s.Name.Last, 1, 64) || !manifestASCII(s.Name.Display, 3, 129) || s.Name.Display != s.Name.First+" "+s.Name.Last || !oneOf(s.Style, "slugger", "outfighter", "boxer_puncher", "pressure", "boxer") || !oneOf(s.Rarity, "club", "professional", "contender", "elite", "generational") || !oneOf(s.EligibilityMode, "current-parent", "canonical-winning-block") {
		return false
	}
	p := s.Physical
	if p.Weight < 128 || p.Weight > 268 || p.Height < 50 || p.Height > 90 || p.Reach < p.Height-2 || p.Reach > p.Height+4 || !oneOf(p.Stance, "orthodox", "southpaw") {
		return false
	}
	class := "heavy"
	switch {
	case p.Weight <= 145:
		class = "light"
	case p.Weight <= 170:
		class = "welter"
	case p.Weight <= 195:
		class = "middle"
	case p.Weight <= 225:
		class = "light_heavy"
	}
	if p.Class != class || s.TalentBudget < 250 || s.TalentBudget > 489 {
		return false
	}
	total := 0
	for _, n := range []int{s.Attributes.Power, s.Attributes.Speed, s.Attributes.Chin, s.Attributes.Stamina, s.Attributes.Skill} {
		total += n
		if n < 1 || n > 100 {
			return false
		}
	}
	return total == s.TalentBudget
}
func validManifestPortrait(p *pilot.ManifestPortrait) bool {
	if p.Version != 1 || !validManifestView(p.View) || p.Sequence == 0 || p.Sequence > 1000 || !publicHex.MatchString(p.ID) || !publicHex.MatchString(p.ManifestDigest) || !publicHex.MatchString(p.PortraitDigest) || len(p.SVG) == 0 || len(p.SVG) > 32768 {
		return false
	}
	for i := range len(p.SVG) {
		if p.SVG[i] > 127 || p.SVG[i] < 32 && p.SVG[i] != 10 && p.SVG[i] != 9 {
			return false
		}
	}
	h := sha256.Sum256(append([]byte("SLUG fighter portrait SVG\x00v1\x00"), []byte(p.SVG)...))
	return hex.EncodeToString(h[:]) == p.PortraitDigest
}
func validManifestReceipt(p *pilot.ManifestReceipt) bool {
	r := p.Record
	if p.Version != 1 || !validManifestView(p.View) || r.Version != registry.Version5 || r.Sequence == 0 || r.Sequence > 1000 || r.Job.AuthorityID != p.View.AuthorityPin.AuthorityID || r.Job.DescriptorID != p.View.AuthorityPin.DescriptorID || r.StaticBinding.DescriptorID != r.Job.DescriptorID || r.Context != r.Job.Context || r.Context.Network != p.View.AuthorityPin.Network || r.Context.Genesis != p.View.AuthorityPin.Genesis || prospectjob.ValidateManifest(r.Job) != nil {
		return false
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > registry.MaxRecordSize {
		return false
	}
	proof, e := json.Marshal(r.Proof)
	if e != nil {
		return false
	}
	if _, e = prospect.ParseProof(proof); e != nil {
		return false
	}
	auth, e := json.Marshal(r.Authorization)
	if e != nil {
		return false
	}
	if _, e = gymauth.ParseAuthorization(auth); e != nil {
		return false
	}
	return r.Proof.GymID == r.Job.GymID && r.Authorization.PublicKey == r.Job.PublicKey && r.Birth.BornAtMS == r.Job.IssuedAtMS && r.Birth.AcceptedAtMS >= r.Job.IssuedAtMS && r.Birth.AcceptedAtMS < r.Job.ExpiresAtMS
}
func manifestStatusCode(s string) bool {
	// Final worker/engine vocabulary is intentionally closed at this pipe.
	return oneOf(s, "", "initialization_failed", "storage_failed", "storage_close_failed", "chain_data_missing", "node_start_failed", "node_shutdown_failed", "wallet_unavailable", "wallet_shutdown_failed", "wallet_connection_failed", "wallet_payout_failed", "wallet_identity_mismatch", "wallet_status_failed", "node_unavailable", "authority_unavailable", "authority_identity_mismatch", "authority_port_busy", "wallet_signer_unavailable", "worker_unavailable", "worker_failed", "node_exited", "authority_exited", "observer_unavailable", "signer_unavailable", "clock", "stale", "capacity", "unavailable", "outcome_unknown", "unauthorized", "forbidden", "rejected", "unknown_job", "busy", "rate_limited", "expired", "invalid_request", "too_large")
}
