package main

import (
	"encoding/json"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/desktop/pooloperator"
	"github.com/taytr-llc/slug/poolingress"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
	"reflect"
	"strings"
	"time"
)

const actualReferenceSHA = "a48a854d20298fced726b816d11690aa45e9f4184e3b161cbc7c258b74897386"
const baseReferenceSHA = "a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3"

type publicFixture struct {
	Version       uint32                       `json:"version"`
	Fixture       string                       `json:"fixture"`
	Options       pilot.PoolOptions            `json:"options"`
	ExpectedLocal pilot.RemotePoolLocalBinding `json:"expected_local"`
	Recipients    []pilot.PoolRecipientBinding `json:"recipients"`
}
type economicBranch struct {
	AcceptedRewards int   `json:"accepted_rewards"`
	Credits         int   `json:"credits"`
	Batches         int   `json:"batches"`
	InputCount      int   `json:"input_count"`
	OutputCount     int   `json:"output_count"`
	GrossAtoms      int64 `json:"gross_atoms"`
	MinerAtoms      int64 `json:"miner_atoms"`
	FeeAtoms        int64 `json:"fee_atoms"`
	ChangeAtoms     int64 `json:"change_atoms"`
}
type actualReference struct {
	Version         uint32
	Fixture         publicFixture
	Setup           pooloperator.Setup
	SetupHex        string `json:"setup_hex"`
	SetupID         string `json:"setup_id"`
	BaseID          string `json:"base_composition_setup_id"`
	Remote          pilot.RemotePoolConfig
	RemoteHex       string `json:"remote_hex"`
	RemoteID        string `json:"remote_id"`
	Worker          prospectworker.PoolConfig
	BlockTarget     string `json:"block_target"`
	Economics       []economicBranch
	OrdinaryMaximum uint64 `json:"ordinary_maximum"`
}

func loadBaseSelection(path, base string) (actualReference, string) {
	var x actualReference
	raw := readBound(path, 262144)
	require(len(raw) == 46498 && hx(digest(raw)) == actualReferenceSHA, "qualified independent actual corpus")
	check(json.Unmarshal(raw, &x), "actual reference")
	s, e := pooloperator.ParseDailySetup(dec(x.SetupHex))
	check(e, "independent exact setup")
	require(reflect.DeepEqual(s, x.Setup), "setup original bytes")
	id, e := pooloperator.DailySetupID(s)
	check(e, "setup ID")
	require(id == x.SetupID, "independent setup ID")
	c, e := pilot.ParseDailyRemotePoolConfig(dec(x.RemoteHex))
	check(e, "independent remote exact config")
	require(reflect.DeepEqual(c, x.Remote), "remote exact bytes")
	cid, e := pilot.DailyRemotePoolConfigID(c)
	check(e, "remote ID")
	require(cid == x.RemoteID && c.ExpectedLocal == x.Fixture.ExpectedLocal, "independent original local selection")
	require(x.Version == 1 && x.Fixture.Fixture == "incrementing256" && x.Fixture.Options.CandidateDescriptorID == "c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303", "original name version1 and unheld fixture")
	reservation, e := pooloperator.DailyCapacityEnvelope(s)
	check(e, "actual reservation")
	require(reservation.OrdinaryMaximum == 2 && reservation.OrdinaryMaximum == x.OrdinaryMaximum, "original true cap2")
	require(s.Pool.PoolTarget == x.BlockTarget && x.BlockTarget == "7fffff"+strings.Repeat("00", 29), "exact real compact target")
	b := readBound(base, 4<<20)
	require(len(b) == 3176583 && hx(digest(b)) == baseReferenceSHA, "qualified original public seed reference")
	var refs struct {
		Composition struct {
			Positives []struct {
				Name   string
				ID     string `json:"setup_id"`
				Backup string `json:"published_fixture_backup_hex"`
			}
		}
	}
	check(json.Unmarshal(b, &refs), "original fixture reference")
	for _, r := range refs.Composition.Positives {
		if r.Name == "two-enrollments" {
			var backup struct {
				Version  uint32
				SetupID  string `json:"setup_id"`
				Mnemonic string
			}
			check(json.Unmarshal(dec(r.Backup), &backup), "published synthetic backup")
			require(r.ID == x.BaseID && backup.Version == 2 && backup.SetupID == r.ID && backup.Mnemonic != "", "same published funding root")
			return x, backup.Mnemonic
		}
	}
	panic("missing independent funding fixture")
}

type recoveryReference struct {
	Version                 uint32
	Base                    actualReference `json:"base_actual"`
	Setup                   pooloperator.RecoverableDailySetup
	SetupID                 string `json:"setup_id"`
	Remote                  pilot.RecoverableRemotePoolConfig
	RemoteID                string `json:"remote_id"`
	Profile                 registry.IssueProfile
	ProfileID               string `json:"profile_id"`
	Worker                  prospectworker.RecoverablePoolConfig
	RequiredAcceptedRewards int            `json:"required_accepted_rewards"`
	SelectedEconomics       economicBranch `json:"selected_economics"`
}

func loadSelection(path, expectedHash, oldPath, base string) (recoveryReference, string) {
	old, mnemonic := loadBaseSelection(oldPath, base)
	raw := readBound(path, 1<<20)
	require(expectedHash == "17557b28921290461fdf47b9ed6f1b800d3d077ee5d325a4ec59277ab3faca6a" && len(raw) == 174293 && hx(digest(raw)) == expectedHash, "root-qualified independent Set81 selection")
	var v recoveryReference
	check(json.Unmarshal(raw, &v), "wrapper reference")
	require(old.Setup.MaintenanceIntervalMS == 30000 && v.Base.Setup.MaintenanceIntervalMS == 1000, "one independently selected maintenance interval")
	expected := old
	expected.Setup.MaintenanceIntervalMS = 1000
	selectedRaw, e := pooloperator.MarshalDailySetup(expected.Setup)
	check(e, "selected base setup grammar")
	expected.SetupHex = hx(selectedRaw)
	expected.SetupID, e = pooloperator.DailySetupID(expected.Setup)
	check(e, "selected base setup identity")
	require(v.Version == 1 && reflect.DeepEqual(v.Base, expected), "only selected interval and dependent setup bytes changed")
	require(reflect.DeepEqual(v.Setup.Base, expected.Setup) && reflect.DeepEqual(v.Remote.Base, old.Remote) && v.Profile == v.Setup.IssueProfile && v.Profile == v.Remote.IssueProfile, "independent original base/profile")
	require(v.RequiredAcceptedRewards == 2 && reflect.DeepEqual(v.SelectedEconomics, selectedEconomics(old, 2)) && v.SelectedEconomics.MinerAtoms == 1980000000 && v.SelectedEconomics.FeeAtoms == 209 && v.SelectedEconomics.ChangeAtoms == 19999791, "mandatory independently derived two-reward economics")
	id, e := pooloperator.RecoverableDailySetupID(v.Setup)
	check(e, "wrapper setup")
	require(id == v.SetupID, "independent setup wrapper ID")
	id, e = pilot.RecoverableRemotePoolConfigID(v.Remote)
	check(e, "wrapper remote")
	require(id == v.RemoteID, "independent remote wrapper ID")
	id, e = registry.IssueProfileID(v.Profile)
	check(e, "wrapper profile")
	require(id == v.ProfileID, "independent profile ID")
	require(reflect.DeepEqual(recoveryWorkerConfig(v.Remote), v.Worker), "independent complete worker wrapper")
	return v, mnemonic
}
func actualSelect(x recoveryReference, private string, started time.Time) (pooloperator.RecoverableDailySetup, pilot.RecoverableRemotePoolConfig, int64) {
	s := x.Setup.Base
	c := x.Remote.Base
	now := started.UTC()
	due := now.Add(7 * time.Minute).Truncate(time.Minute)
	require(now.Add(15*time.Minute).Format("2006-01-02") == now.Format("2006-01-02"), "same-day finite actual")
	s.Schedule.DailyUTCMinute = uint32(due.Hour()*60 + due.Minute())
	ca, _, spki := fixtureTLS(private)
	p := uniquePorts(5)
	// One immutable external Origin and certificate selection. Only the backend
	// listener differs; relay preserves the original Host and TLS identity.
	s.Ingress = poolingress.Config{Version: 2, Listen: p[1], Origin: "https://" + p[0], CertificateSPKI: spki, PoolURL: "http://" + p[2], AdmissionURL: "http://" + p[3], OwnershipURL: "http://" + p[4]}
	check(pooloperator.ValidateDailySetup(s), "immutable runtime transport/due")
	c.Origin = s.Ingress.Origin
	c.CACertificate = ca
	check(pilot.ValidateDailyRemotePoolConfig(c), "original local selection with runtime TLS")
	normalized := s
	normalized.Ingress = x.Setup.Base.Ingress
	normalized.Schedule.DailyUTCMinute = x.Setup.Base.Schedule.DailyUTCMinute
	require(reflect.DeepEqual(normalized, x.Setup.Base), "no financial or source mutation")
	sr, e := pooloperator.ComposeRecoverableDailySetup(s)
	check(e, "selected actual wrapper")
	cr := pilot.RecoverableRemotePoolConfig{Version: 1, Base: c, IssueProfile: sr.IssueProfile}
	check(pilot.ValidateRecoverableRemotePoolConfig(cr), "selected actual remote")
	return sr, cr, due.UnixMilli()
}
func clientConfig(s pooloperator.Setup, c pilot.RemotePoolConfig, index int, token string) poolservice.DailyClientConfig {
	e := s.Miners.Enrollments[index]
	return poolservice.DailyClientConfig{Version: 2, URL: c.Origin, Token: token, CACertificate: c.CACertificate, PublicKey: e.PublicKey, Source: s.Pool, SettlementConfigID: s.Miners.SettlementConfigID, RecipientScript: e.PayeeScript}
}
func workerConfig(c pilot.RemotePoolConfig) prospectworker.PoolConfig {
	sid, e := poolsettle.DailyConfigBindingID(c.Settlement)
	check(e, "worker settlement ID")
	key := c.ExpectedLocal.Recipient.Config.Identity.GymPublicKey
	pc := poolservice.DailyClientConfig{Version: 2, URL: c.Origin, Token: strings.Repeat("1", 64), CACertificate: c.CACertificate, PublicKey: key, Source: c.Source, SettlementConfigID: sid, RecipientScript: c.ExpectedLocal.Recipient.Recipient.Script}
	pt, e := poolservice.DailyTransportTrustID(pc)
	check(e, "daily trust")
	ac := prospectservice.ManifestClientConfig{Version: 3, URL: c.Origin, Token: strings.Repeat("1", 64), CACertificate: c.CACertificate, AuthorityPin: c.Source.AuthorityPin}
	at, e := prospectservice.ManifestTransportTrustID(ac)
	check(e, "admission trust")
	w := prospectworker.PoolConfig{Version: 2, Source: c.Source, PublicKey: key, AuthorityOrigin: c.Origin, TransportTrustID: pt, AdmissionOrigin: c.Origin, AdmissionTrustID: at, MaxRecords: c.MaxRecords, MaxBytes: c.MaxBytes}
	_, e = prospectworker.ValidateDailyPoolConfig(w)
	check(e, "actual worker config")
	return w
}

func recoveryClientConfig(s pooloperator.RecoverableDailySetup, c pilot.RecoverableRemotePoolConfig, index int, token string) poolservice.RecoverableDailyClientConfig {
	return poolservice.RecoverableDailyClientConfig{Version: 1, Base: clientConfig(s.Base, c.Base, index, token), IssueProfile: s.IssueProfile}
}
func recoveryWorkerConfig(c pilot.RecoverableRemotePoolConfig) prospectworker.RecoverablePoolConfig {
	b := workerConfig(c.Base)
	sid, e := poolsettle.DailyConfigBindingID(c.Base.Settlement)
	check(e, "selected financial ID")
	key := c.Base.ExpectedLocal.Recipient.Config.Identity.GymPublicKey
	cc := poolservice.RecoverableDailyClientConfig{Version: 1, Base: poolservice.DailyClientConfig{Version: 2, URL: c.Base.Origin, Token: strings.Repeat("1", 64), CACertificate: c.Base.CACertificate, PublicKey: key, Source: c.Base.Source, SettlementConfigID: sid, RecipientScript: c.Base.ExpectedLocal.Recipient.Recipient.Script}, IssueProfile: c.IssueProfile}
	id, e := poolservice.RecoverableDailyTransportTrustID(cc)
	check(e, "selected complete transport trust")
	w := prospectworker.RecoverablePoolConfig{Version: 1, Base: b, IssueProfile: c.IssueProfile, SettlementConfigID: sid, RecipientScript: cc.Base.RecipientScript, TransportTrustID: id}
	_, e = prospectworker.ValidateRecoverableDailyPoolConfig(w)
	check(e, "selected original worker")
	return w
}
