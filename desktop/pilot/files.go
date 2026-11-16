package pilot

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectworker"
)

const fixturePublicKey = "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"
const fixturePayout = "amkchND9pP6STAzbb4M6U4SzU2anReC4R4"
const fixtureToken = "4242424242424242424242424242424242424242424242424242424242424242"
const genesis = "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd"
const target = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

type manifest struct {
	Version       uint32 `json:"version"`
	Mode          string `json:"mode"`
	AuthorityPort uint16 `json:"authority_port"`
}
type savedStatus struct {
	Height   uint64 `json:"height"`
	Attempts uint64 `json:"attempts"`
}

func policy() prospectnode.Policy {
	return prospectnode.Policy{Version: 1, Network: 1397511495, Genesis: genesis, DNAVersion: 1, ProspectTarget: target}
}
func (m manifest) origin() string { return "http://127.0.0.1:" + strconv.Itoa(int(m.AuthorityPort)) }
func (m manifest) workerConfig() prospectworker.Config {
	return prospectworker.Config{Version: 1, Policy: policy(), PublicKey: fixturePublicKey, PayoutAddress: fixturePayout, AuthorityOrigin: m.origin(), MaxRecords: 1000, MaxBytes: 64 << 20}
}
func readJSON(path string, limit int64, v any) error {
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() {
		return errors.New("invalid pilot data file")
	}
	f, e := os.Open(path)
	if e != nil {
		return errors.New("pilot data unavailable")
	}
	defer f.Close()
	info, e = f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return errors.New("invalid pilot data file")
	}
	raw, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(raw)) > limit {
		return errors.New("invalid pilot data size")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(v); e != nil {
		return errors.New("invalid pilot data")
	}
	canonical, err := json.Marshal(v)
	if err != nil || !bytes.Equal(bytes.TrimSpace(raw), canonical) {
		return errors.New("noncanonical pilot data")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("invalid pilot data")
	}
	return nil
}
func atomicJSON(path string, v any) error {
	raw, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return atomicBytes(path, append(raw, '\n'))
}
func atomicBytes(path string, raw []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".pilot-write-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(raw)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	return syncDirectory(filepath.Dir(path))
}
func unusedPort() (uint16, error) {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return 0, e
	}
	p := uint16(l.Addr().(*net.TCPAddr).Port)
	e = l.Close()
	return p, e
}

func syncDirectory(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func plainChild(path string, directory bool) error {
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) {
		return errors.New("invalid managed pilot path")
	}
	return nil
}

// Chain roots exist before the first Start. Once both nodes have initialized,
// retain that fact separately so a missing database can never become a new
// genesis chain on a later Start. Node startup validates the database contents.
func requireChainFiles(root string) error {
	for _, name := range []string{"primary-chain", "peer-chain"} {
		base := filepath.Join(root, name, "mainnet", "blocks_ffldb")
		for _, path := range []string{filepath.Join(root, name), filepath.Join(root, name, "mainnet"), base, filepath.Join(base, "metadata")} {
			info, e := os.Lstat(path)
			if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("pilot chain data is missing")
			}
		}
		for _, path := range []string{filepath.Join(base, "metadata", "CURRENT"), filepath.Join(base, "000000000.fdb")} {
			info, e := os.Lstat(path)
			if e != nil || !info.Mode().IsRegular() {
				return errors.New("pilot chain data is missing")
			}
		}
	}
	return nil
}

func checkInitializedChains(root string, identity manifest) error {
	path := filepath.Join(root, "SLUG-PILOT-CHAINS.json")
	if _, e := os.Lstat(path); os.IsNotExist(e) {
		return nil
	}
	var saved manifest
	if readJSON(path, 1024, &saved) != nil || saved != identity {
		return errors.New("invalid pilot chain identity")
	}
	return requireChainFiles(root)
}

func markInitializedChains(root string, identity manifest) error {
	path := filepath.Join(root, "SLUG-PILOT-CHAINS.json")
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		return checkInitializedChains(root, identity)
	}
	if e := requireChainFiles(root); e != nil {
		return e
	}
	return atomicJSON(path, identity)
}
