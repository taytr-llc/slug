// Package pilot owns a disposable, loopback-only mining environment. Its public
// fixture identity is never a production wallet or a source of real assets.
package pilot

import "github.com/taytr-llc/slug/fighter"

const Mode = "local-pilot"
const MaxRequestSize = 4096
const MaxResponseSize = 256 << 10

type Config struct {
	DataDir    string
	NodeBinary string
}
type Prospect struct {
	Sequence     uint64             `json:"sequence"`
	ID           string             `json:"id"`
	WorkHash     string             `json:"work_hash"`
	BornAtMS     int64              `json:"born_at_ms"`
	Rarity       string             `json:"rarity"`
	TalentBudget int                `json:"talent_budget"`
	Attributes   fighter.Attributes `json:"attributes"`
}
type Snapshot struct {
	State           string     `json:"state"`
	Mode            string     `json:"mode"`
	Attempts        uint64     `json:"attempts"`
	HashesPerSecond float64    `json:"hashes_per_second"`
	Height          uint64     `json:"height"`
	Pending         uint64     `json:"pending"`
	LastError       string     `json:"last_error"`
	RecoveryError   string     `json:"recovery_error"`
	ProspectCount   uint64     `json:"prospect_count"`
	Prospects       []Prospect `json:"prospects"`
}
