//go:build !slugtestpow

package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("SLUG_PILOT_TEST_HELPER") == "1" && len(os.Args) == 2 {
		if os.Args[1] == "--version" {
			fmt.Println("slugd version 0.27.0")
			os.Exit(0)
		}
		if strings.HasPrefix(os.Args[1], "--configfile=") {
			os.Exit(2)
		}
	}
	os.Exit(m.Run())
}
func testConfig(t *testing.T) Config {
	t.Helper()
	t.Setenv("SLUG_PILOT_TEST_HELPER", "1")
	self, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	return Config{DataDir: filepath.Join(t.TempDir(), "pilot"), NodeBinary: self}
}
func testController(t *testing.T) (*Controller, Config) {
	t.Helper()
	config := testConfig(t)
	c, e := New(config)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, config
}
func TestNewPreservesIdentityAndDoesNotStartMining(t *testing.T) {
	c, config := testController(t)
	origin := c.manifest.origin()
	if s := c.Snapshot(); s.State != "stopped" || s.Mode != Mode || s.Attempts != 0 || s.ProspectCount != 0 {
		t.Fatalf("unexpected initial state: %+v", s)
	}
	if entries, e := os.ReadDir(filepath.Join(config.DataDir, "primary-chain")); e != nil || len(entries) != 0 {
		t.Fatal("constructor started a node")
	}
	if _, e := New(config); e == nil {
		t.Fatal("opened same pilot twice")
	}
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := New(config)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if reopened.manifest.origin() != origin {
		t.Fatal("restart changed authority identity")
	}
}
func TestManifestRejectsAmbiguousSchema(t *testing.T) {
	for _, kind := range []string{"duplicate", "case alias", "null", "unknown", "wrong mode", "zero port", "missing"} {
		t.Run(kind, func(t *testing.T) {
			c, config := testController(t)
			c.Close()
			path := filepath.Join(config.DataDir, "SLUG-PILOT.json")
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			s := string(raw)
			switch kind {
			case "duplicate":
				s = strings.Replace(s, `"version":1`, `"version":1,"version":1`, 1)
			case "case alias":
				s = strings.Replace(s, `"version"`, `"Version"`, 1)
			case "null":
				s = strings.Replace(s, `"version":1`, `"version":null`, 1)
			case "unknown":
				s = strings.Replace(s, `"version":1`, `"version":1,"secret":1`, 1)
			case "wrong mode":
				s = strings.Replace(s, Mode, "production", 1)
			case "zero port":
				var m manifest
				json.Unmarshal(raw, &m)
				s = strings.Replace(s, fmt.Sprint(m.AuthorityPort), "0", 1)
			case "missing":
				s = strings.Replace(s, `"version":1,`, "", 1)
			}
			if e := os.WriteFile(path, []byte(s), 0600); e != nil {
				t.Fatal(e)
			}
			if _, e := New(config); e == nil {
				t.Fatal("ambiguous manifest accepted")
			}
			after, _ := os.ReadFile(path)
			if string(after) != s {
				t.Fatal("invalid evidence was overwritten")
			}
		})
	}
}
func TestUnrelatedDirectoryAndSymlinkAreRefused(t *testing.T) {
	config := testConfig(t)
	if e := os.Mkdir(config.DataDir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(config.DataDir, "user.txt"), []byte("preserve"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := New(config); e == nil {
		t.Fatal("unrelated directory adopted")
	}
	if _, e := os.Stat(filepath.Join(config.DataDir, "SLUG-PILOT.json")); !os.IsNotExist(e) {
		t.Fatal("unrelated directory changed")
	}
}
func TestConcurrentStartStopAndFailedChildCleanup(t *testing.T) {
	c, _ := testController(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = c.Start() }()
	}
	wg.Wait()
	c.Stop()
	_ = c.Close() // The fixture child deliberately exits unsuccessfully.
	if s := c.Snapshot(); s.State != "stopped" && s.State != "failed" {
		t.Fatalf("not drained: %+v", s)
	}
}
func TestProtocolRejectsMalformedAndUnboundedRequests(t *testing.T) {
	invalid := []string{`{}`, `[]`, `{"id":1,"id":1,"command":"start"}`, `{"id":1,"command":null}`, `{"id":1,"Command":"start"}`, `{"id":1,"command":"shell"}`, `{"id":1,"command":"receipt"}`, `{"id":1,"command":"receipt","sequence":0}`, `{"id":1,"command":"snapshot","sequence":1}`, `{"id":9007199254740992,"command":"snapshot"}`, `{"id":1,"command":"snapshot"} {}`, strings.Repeat(" ", MaxRequestSize+1)}
	for _, raw := range invalid {
		if _, e := parseRequest([]byte(raw)); e == nil {
			t.Errorf("accepted invalid request %q", raw)
		}
	}
	if got, e := parseRequest([]byte(`{"id":12,"command":"receipt","sequence":1}`)); e != nil || got.ID != 12 || got.Sequence != 1 {
		t.Fatalf("valid request: %+v %v", got, e)
	}
}
func TestProtocolEOFReleasesPilotAndRedactsErrors(t *testing.T) {
	c, config := testController(t)
	var output bytes.Buffer
	input := strings.NewReader("{\"id\":1,\"command\":\"snapshot\"}\n{\"id\":2,\"command\":\"receipt\",\"sequence\":1}\n")
	if e := Serve(context.Background(), input, &output, c); e != nil {
		t.Fatal(e)
	}
	var responses []Response
	d := json.NewDecoder(&output)
	for {
		var response Response
		e := d.Decode(&response)
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		responses = append(responses, response)
	}
	if len(responses) != 2 || !responses[0].OK || responses[0].Snapshot.State != "stopped" || responses[1].OK || responses[1].Error != "receipt_unavailable" {
		t.Fatalf("responses: %+v", responses)
	}
	reopened, e := New(config)
	if e != nil {
		t.Fatal("EOF did not release lock", e)
	}
	reopened.Close()
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("fixture output failure") }
func TestProtocolOutputFailureDrains(t *testing.T) {
	c, config := testController(t)
	if e := Serve(context.Background(), strings.NewReader("{\"id\":1,\"command\":\"snapshot\"}\n"), failingWriter{}, c); e == nil {
		t.Fatal("missing output error")
	}
	reopened, e := New(config)
	if e != nil {
		t.Fatal(e)
	}
	reopened.Close()
}
func TestProtocolCancellationClosesIdlePipe(t *testing.T) {
	c, _ := testController(t)
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, reader, io.Discard, c) }()
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled idle reader did not drain")
	}
}

func TestConcurrentFirstLaunchNeverReplacesIdentity(t *testing.T) {
	config := testConfig(t)
	const count = 8
	results := make(chan *Controller, count)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c, err := New(config)
			if err == nil {
				results <- c
			}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var winner *Controller
	for c := range results {
		if winner != nil {
			c.Close()
			t.Fatal("two controllers acquired one pilot")
		}
		winner = c
	}
	if winner == nil {
		t.Fatal("no initialization succeeded")
	}
	origin := winner.manifest.origin()
	if e := winner.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := New(config)
	if e != nil {
		t.Fatal("concurrent launch damaged identity", e)
	}
	defer reopened.Close()
	if reopened.manifest.origin() != origin {
		t.Fatal("winning authority identity was replaced")
	}
}

func TestEstablishedPilotNeverRecreatesMissingEvidence(t *testing.T) {
	for _, name := range []string{"registry", "outbox", "primary-chain", "peer-chain", "SLUG-PILOT-READY.json"} {
		t.Run(name, func(t *testing.T) {
			c, config := testController(t)
			c.Close()
			path := filepath.Join(config.DataDir, name)
			if e := os.Rename(path, path+".retained"); e != nil {
				t.Fatal(e)
			}
			if opened, e := New(config); e == nil {
				opened.Close()
				t.Fatal("missing established evidence recreated")
			}
			if _, e := os.Lstat(path); !os.IsNotExist(e) {
				t.Fatal("failed startup changed missing evidence")
			}
		})
	}
}

func TestManagedSymlinkRefused(t *testing.T) {
	for _, name := range []string{"registry", "outbox", "primary-chain", "peer-chain"} {
		t.Run(name, func(t *testing.T) {
			c, config := testController(t)
			c.Close()
			path := filepath.Join(config.DataDir, name)
			retained := path + ".retained"
			if e := os.Rename(path, retained); e != nil {
				t.Fatal(e)
			}
			if e := os.Symlink(retained, path); e != nil {
				t.Fatal(e)
			}
			if opened, e := New(config); e == nil {
				opened.Close()
				t.Fatal("managed symlink accepted")
			}
		})
	}
}

func TestFailedStatusSavePropagatesThroughClose(t *testing.T) {
	c, config := testController(t)
	if e := os.Mkdir(filepath.Join(config.DataDir, "status.json"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := c.Start(); e != nil {
		t.Fatal(e)
	}
	c.Stop()
	if e := c.Close(); e == nil {
		t.Fatal("failed status save reported clean shutdown")
	}
	if c.Snapshot().State != "failed" {
		t.Fatal("failed status save hidden in snapshot")
	}
}

type blockedOutput struct {
	entered   chan struct{}
	closed    chan struct{}
	enterOnce sync.Once
	closeOnce sync.Once
}

func (w *blockedOutput) Write([]byte) (int, error) {
	w.enterOnce.Do(func() { close(w.entered) })
	<-w.closed
	return 0, io.ErrClosedPipe
}
func (w *blockedOutput) Close() error { w.closeOnce.Do(func() { close(w.closed) }); return nil }

func TestEOFInterruptsBlockedOutputAndDrainsController(t *testing.T) {
	c, config := testController(t)
	input, writer := io.Pipe()
	output := &blockedOutput{entered: make(chan struct{}), closed: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), input, output, c) }()
	if _, e := io.WriteString(writer, "{\"id\":1,\"command\":\"start\"}\n"); e != nil {
		t.Fatal(e)
	}
	select {
	case <-output.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("response never entered writer")
	}
	writer.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("EOF did not interrupt blocked output")
	}
	reopened, e := New(config)
	if e != nil {
		t.Fatal("EOF left controller active", e)
	}
	reopened.Close()
}

func TestInitializedChainLossFailsBeforeNodeLaunch(t *testing.T) {
	for _, lost := range []string{"primary-chain", "peer-chain/mainnet/blocks_ffldb", "primary-chain/mainnet/blocks_ffldb/metadata", "peer-chain/mainnet/blocks_ffldb/metadata/CURRENT", "primary-chain/mainnet/blocks_ffldb/000000000.fdb"} {
		t.Run(lost, func(t *testing.T) {
			c, config := testController(t)
			// Public structural fixtures only. Actual DB validation is tested
			// by the real-node supervisor integration, not these empty files.
			for _, name := range []string{"primary-chain", "peer-chain"} {
				base := filepath.Join(config.DataDir, name, "mainnet", "blocks_ffldb")
				if e := os.MkdirAll(filepath.Join(base, "metadata"), 0700); e != nil {
					t.Fatal(e)
				}
				for _, p := range []string{filepath.Join(base, "metadata", "CURRENT"), filepath.Join(base, "000000000.fdb")} {
					if e := os.WriteFile(p, []byte("structural-test-only"), 0600); e != nil {
						t.Fatal(e)
					}
				}
			}
			if e := markInitializedChains(config.DataDir, c.manifest); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(config.DataDir, filepath.FromSlash(lost))
			if e := os.Rename(path, path+".retained"); e != nil {
				t.Fatal(e)
			}
			if e := c.Start(); e != nil {
				t.Fatal(e)
			}
			c.mu.Lock()
			done := c.done
			c.mu.Unlock()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("chain failure did not terminate")
			}
			if s := c.Snapshot(); s.State != "failed" || s.LastError != "chain_data_missing" {
				t.Fatalf("wrong state: %+v", s)
			}
			if _, e := os.Lstat(filepath.Join(config.DataDir, "peer.conf")); !os.IsNotExist(e) {
				t.Fatal("node was launched after missing chain")
			}
			c.Close()
			if reopened, e := New(config); e == nil {
				reopened.Close()
				t.Fatal("missing chain accepted on reopen")
			}
			if _, e := os.Lstat(path); !os.IsNotExist(e) {
				t.Fatal("missing chain was recreated")
			}
		})
	}
}
