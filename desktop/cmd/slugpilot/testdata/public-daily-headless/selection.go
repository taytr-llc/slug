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

func loadSelection(path, base string) (actualReference, string) {
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
func actualSelect(x actualReference, private string) (pooloperator.Setup, pilot.RemotePoolConfig, int64) {
	s := x.Setup
	c := x.Remote
	now := time.Now().UTC()
	due := now.Add(5 * time.Minute).Truncate(time.Minute)
	require(now.Add(15*time.Minute).Format("2006-01-02") == now.Format("2006-01-02"), "same day actual window")
	s.Schedule.DailyUTCMinute = uint32(due.Hour()*60 + due.Minute())
	ca, _, spki := fixtureTLS(private)
	p := uniquePorts(4)
	s.Ingress = poolingress.Config{Version: 2, Listen: p[0], Origin: "https://" + p[0], CertificateSPKI: spki, PoolURL: "http://" + p[1], AdmissionURL: "http://" + p[2], OwnershipURL: "http://" + p[3]}
	check(pooloperator.ValidateDailySetup(s), "dynamic transport/due only")
	c.Origin = s.Ingress.Origin
	c.CACertificate = ca
	check(pilot.ValidateDailyRemotePoolConfig(c), "exact original local/remote actual config")
	// Only these selected public runtime fields differ from the qualified fixture.
	normalized := s
	normalized.Ingress = x.Setup.Ingress
	normalized.Schedule.DailyUTCMinute = x.Setup.Schedule.DailyUTCMinute
	require(reflect.DeepEqual(normalized, x.Setup), "no other dynamic setup change")
	return s, c, due.UnixMilli()
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
