// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

func chainReferencePin(t *testing.T) Pin {
	t.Helper()
	raw, err := os.ReadFile("testdata/reference/public-core-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "1145dede4fcfa453e8bb1cdffff01079f6cca1df3bbe810dc32868207292b507" {
		t.Fatal("independent source corpus changed")
	}
	var corpus struct {
		Positives []struct {
			Family string          `json:"family"`
			Value  json.RawMessage `json:"value"`
		} `json:"positives"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("independent source reference")
	}
	for _, v := range corpus.Positives {
		if v.Family == "pin" {
			var p Pin
			if json.Unmarshal(v.Value, &p) != nil {
				t.Fatal("pin")
			}
			return p
		}
	}
	t.Fatal("missing independent pin")
	return Pin{}
}

type chainFake struct {
	mu                   sync.Mutex
	pin                  Pin
	observation          ChainObservation
	obsCalls             int
	busy                 int
	errorCode            int
	errorMessage         string
	malformedObservation string
	wrongNetwork         bool
	blockEntered         chan struct{}
	blockCancelled       chan struct{}
	blockRaw             json.RawMessage
	txoutRaw             json.RawMessage
	txoutAdvances        bool
}

func (f *chainFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID     uint64            `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request) != nil {
		http.Error(w, "invalid", 400)
		return
	}
	f.mu.Lock()
	var result any
	var rpcError any
	switch request.Method {
	case "getcurrentnet":
		result = f.pin.Source.Network
		if f.wrongNetwork {
			result = uint32(1)
		}
	case "getblockhash":
		var height uint32
		if len(request.Params) != 1 || json.Unmarshal(request.Params[0], &height) != nil {
			f.mu.Unlock()
			http.Error(w, "invalid", 400)
			return
		}
		result = f.pin.Source.Genesis
		if height != 0 {
			result = strings.Repeat("1", 64)
		}
	case "getchainobservation":
		f.obsCalls++
		result = f.observation
		if f.malformedObservation != "" {
			result = json.RawMessage(f.malformedObservation)
		}
		if f.busy > 0 {
			f.busy--
			result = nil
			rpcError = struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{-32603, "chain observation busy"}
		}
		if f.errorMessage != "" {
			result = nil
			rpcError = struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{f.errorCode, f.errorMessage}
		}
	case "getblockheader":
		result = strings.Repeat("00", 80)
	case "getblock":
		if f.blockEntered != nil {
			entered, cancelled := f.blockEntered, f.blockCancelled
			f.blockEntered = nil
			f.mu.Unlock()
			close(entered)
			<-r.Context().Done()
			close(cancelled)
			return
		}
		result = strings.Repeat("00", 81)
		if f.blockRaw != nil {
			result = f.blockRaw
		}
	case "getrawtransaction":
		result = "01000000000000000000"
	case "gettxout":
		if len(request.Params) != 3 || string(request.Params[0]) != `"`+strings.Repeat("4", 64)+`"` || string(request.Params[1]) != "0" || string(request.Params[2]) != "true" {
			f.mu.Unlock()
			http.Error(w, "invalid exact unspent request", 400)
			return
		}
		result = f.txoutRaw
		if f.txoutAdvances {
			f.observation.Revision = "2"
		}
	default:
		f.mu.Unlock()
		http.Error(w, "invalid", 400)
		return
	}
	override := f.malformedObservation
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if request.Method == "getchainobservation" && override != "" && rpcError == nil {
		_, _ = io.WriteString(w, `{"result":`+override+`,"error":null,"id":`+strconv.FormatUint(request.ID, 10)+`}`)
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		Result any    `json:"result"`
		Error  any    `json:"error"`
		ID     uint64 `json:"id"`
	}{result, rpcError, request.ID})
}
func chainServer(t *testing.T) (*OwnedChainSource, *chainFake) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("owned source requires normal profile")
	}
	pin := chainReferencePin(t)
	f := &chainFake{pin: pin, observation: ChainObservation{1, strings.Repeat("2", 64), "1", strings.Repeat("1", 64), 2}}
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	s, err := NewOwnedChainSource(context.Background(), pin, prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "public-fixture", Password: "public-fixture"}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, f
}
func TestOwnedChainSourceObservationAndBytes(t *testing.T) {
	s, f := chainServer(t)
	before, err := s.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Check(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.observation.Revision = "3"
	f.mu.Unlock()
	if err = s.Check(context.Background(), before); !errors.Is(err, ErrStale) {
		t.Fatal("same hash ABA not refused", err)
	}
	f.mu.Lock()
	f.observation.Revision = "1"
	f.observation.InstanceID = strings.Repeat("3", 64)
	f.mu.Unlock()
	if err = s.Check(context.Background(), before); !errors.Is(err, ErrStale) {
		t.Fatal("restart not refused", err)
	}
	header, err := s.HeaderAt(context.Background(), 2)
	if err != nil || header.Height != 2 || header.Hash != strings.Repeat("1", 64) || len(header.Raw) != 80 {
		t.Fatal("raw header transport", err)
	}
	block, err := s.Block(context.Background(), header.Hash)
	if err != nil || len(block) != 81 {
		t.Fatal("raw block transport", err)
	}
	tx, err := s.Transaction(context.Background(), strings.Repeat("4", 64))
	if err != nil || len(tx) != 10 {
		t.Fatal("raw transaction transport", err)
	}
	block[0] = 255
	again, err := s.Block(context.Background(), header.Hash)
	if err != nil || again[0] != 0 {
		t.Fatal("borrowed bytes", err)
	}
	// Raw return is intentionally not a chain-validity verdict; core replay
	// independently verifies complete headers, transaction framing and Merkle.
	f.mu.Lock()
	f.wrongNetwork = true
	f.mu.Unlock()
	if _, err = s.Transaction(context.Background(), strings.Repeat("4", 64)); !errors.Is(err, ErrSource) {
		t.Fatal("identity not rechecked", err)
	}
}
func TestOwnedChainSourceRetriesOnlyExactBusy(t *testing.T) {
	s, f := chainServer(t)
	f.mu.Lock()
	f.busy = 2
	f.mu.Unlock()
	if _, err := s.Observe(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	count := f.obsCalls
	f.mu.Unlock()
	if count != 3 {
		t.Fatal("busy retry count", count)
	}
	for _, c := range []struct {
		code    int
		message string
	}{{-32603, "chain observation unavailable"}, {-32602, "chain observation busy"}, {-32603, "chain observation busy "}} {
		f.mu.Lock()
		f.errorCode = c.code
		f.errorMessage = c.message
		before := f.obsCalls
		f.mu.Unlock()
		if _, err := s.Observe(context.Background()); !errors.Is(err, ErrUnavailable) {
			t.Fatal("unrelated error", err)
		}
		f.mu.Lock()
		after := f.obsCalls
		f.mu.Unlock()
		if after != before+1 {
			t.Fatal("unrelated error retried")
		}
	}
}
func TestOwnedChainSourceRejectsMalformedAndCapacity(t *testing.T) {
	s, f := chainServer(t)
	raw, _ := json.Marshal(f.observation)
	for _, bad := range []string{strings.Replace(string(raw), `"version":1`, `"version": 1`, 1), strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1), strings.Replace(string(raw), `"revision":"1"`, `"revision":"01"`, 1), strings.Replace(string(raw), `"height":2`, `"height":2147483648`, 1)} {
		f.mu.Lock()
		f.malformedObservation = bad
		f.mu.Unlock()
		if _, err := s.Observe(context.Background()); err == nil {
			t.Fatal("malformed observation accepted")
		}
	}
	f.mu.Lock()
	f.malformedObservation = ""
	f.blockRaw = json.RawMessage(`"` + strings.Repeat("0", 2*wire.MaxBlockPayload+2) + `"`)
	f.mu.Unlock()
	if _, err := s.Block(context.Background(), strings.Repeat("1", 64)); !errors.Is(err, ErrCapacity) {
		t.Fatal("oversize", err)
	}
	f.mu.Lock()
	f.blockRaw = json.RawMessage(`"\u0030\u0030"`)
	f.mu.Unlock()
	if _, err := s.Block(context.Background(), strings.Repeat("1", 64)); !errors.Is(err, ErrUnavailable) {
		t.Fatal("escaped raw hex accepted", err)
	}
	s.nextID.Store(math.MaxUint64)
	if _, err := s.Observe(context.Background()); !errors.Is(err, ErrCapacity) {
		t.Fatal("request counter overflow", err)
	}
	if err := s.Alive(); !errors.Is(err, ErrClosed) {
		t.Fatal("overflow source remains live", err)
	}
}
func TestOwnedChainSourceCloseCancelsAndJoins(t *testing.T) {
	s, f := chainServer(t)
	entered, cancelled := make(chan struct{}), make(chan struct{})
	f.mu.Lock()
	f.blockEntered = entered
	f.blockCancelled = cancelled
	f.mu.Unlock()
	result := make(chan error, 1)
	go func() { _, err := s.Block(context.Background(), strings.Repeat("1", 64)); result <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("call never entered")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("closed request returned data")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not join")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("server transport remained active")
	}
	if _, err := s.Observe(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatal("closed source reopened", err)
	}
}
func TestOwnedChainSourceInputAndProfileRefusals(t *testing.T) {
	var zero OwnedChainSource
	if err := zero.Alive(); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := zero.Observe(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	pin := chainReferencePin(t)
	if wire.TestPoW {
		if _, err := NewOwnedChainSource(context.Background(), pin, prospectnode.RPCConfig{}, context.Background()); !errors.Is(err, ErrProfile) {
			t.Fatal("cheap source accepted", err)
		}
		return
	}
	for _, endpoint := range []string{"http://localhost:1", "http://192.0.2.1:1", "https://example.com", "http://127.0.0.1:1/?x=1"} {
		if _, err := NewOwnedChainSource(context.Background(), pin, prospectnode.RPCConfig{Version: 1, URL: endpoint, Username: "fixture", Password: "fixture"}, context.Background()); err == nil {
			t.Fatal("invalid endpoint accepted")
		}
	}
}
