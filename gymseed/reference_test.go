// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const seedCorpusHash = "2b8d78e8ef665e372543eb87bb5d25a975026a0ae946b15a871ac97bbf2153f0"

type seedReference struct {
	Rules struct{ ID string }
	Pin   struct {
		Value     Pin
		Canonical string
		StoreID   string `json:"store_id"`
	}
	Calendar struct {
		Rows []struct {
			Saturday string
			Cutoff   int64 `json:"cutoff_ms"`
			Deadline int64 `json:"incident_deadline_ms"`
			Event    gymevent.Event
			EventKey string `json:"event_key"`
		}
		Invalid []string `json:"invalid_dates"`
	}
	DTOs []struct {
		Kind, Name, Canonical string
		Value                 json.RawMessage
		Digest                string `json:"record_digest"`
	}
	Cases   []seedRecipe                         `json:"codec_cases"`
	Records []struct{ Digest, Canonical string } `json:"record_states"`
	Scans   []struct {
		Kind       string
		Generation uint32
		EventKey   string `json:"event_key"`
		Initial    string `json:"initial_digest"`
		Steps      []struct {
			Height       uint32
			Hash, Digest string
		}
	} `json:"scan_fold_cases"`
	Scenarios []struct {
		Name  string
		Steps []struct {
			Operation string
			Now       int64 `json:"now_ms"`
			Expected  struct {
				Digest            string `json:"record_digest"`
				FundingState      string `json:"funding_state"`
				FundingGeneration uint32 `json:"funding_generation"`
			}
			Status *Status
		}
	}
}

func readSeedReference(t testing.TB) seedReference {
	t.Helper()
	raw, e := os.ReadFile("testdata/reference/seed_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != seedCorpusHash {
		t.Fatal("frozen corpus changed")
	}
	var c seedReference
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	return c
}

// Ordered finite recipe tree retains the independent input member order. It is
// test-only; every reconstructed input is checked against its frozen SHA first.
type seedJSON struct {
	Kind     byte
	Keys     []string
	Children []*seedJSON
	Scalar   any
}

func seedTree(raw []byte) *seedJSON {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var read func() *seedJSON
	read = func() *seedJSON {
		v, e := d.Token()
		if e != nil {
			panic(e)
		}
		n := &seedJSON{Scalar: v}
		if q, ok := v.(json.Delim); ok {
			n.Kind = byte(q)
			for d.More() {
				if q == '{' {
					k, _ := d.Token()
					n.Keys = append(n.Keys, k.(string))
				}
				n.Children = append(n.Children, read())
			}
			d.Token()
		}
		return n
	}
	return read()
}
func (n *seedJSON) encode(reverse bool) []byte {
	if n.Kind == 0 {
		b, _ := json.Marshal(n.Scalar)
		return b
	}
	var b bytes.Buffer
	b.WriteByte(n.Kind)
	for pos := range n.Children {
		if pos > 0 {
			b.WriteByte(',')
		}
		i := pos
		if reverse && n.Kind == '{' {
			i = len(n.Children) - 1 - pos
		}
		if n.Kind == '{' {
			k, _ := json.Marshal(n.Keys[i])
			b.Write(k)
			b.WriteByte(':')
		}
		b.Write(n.Children[i].encode(reverse))
	}
	if n.Kind == '{' {
		b.WriteByte('}')
	} else {
		b.WriteByte(']')
	}
	return b.Bytes()
}
func (n *seedJSON) child(k string) (int, *seedJSON) {
	if n.Kind == '[' {
		i, e := strconv.Atoi(k)
		if e != nil || i < 0 || i >= len(n.Children) {
			return -1, nil
		}
		return i, n.Children[i]
	}
	for i, key := range n.Keys {
		if k == key {
			return i, n.Children[i]
		}
	}
	return -1, nil
}

type seedRecipe struct {
	Kind, Name, Format string
	DTO                int
	Accepted           bool
	Bytes              int
	InputBytes         int      `json:"input_bytes"`
	InputSHA           string   `json:"input_sha256"`
	Expected           *int     `json:"expected_dto"`
	Replace            []string `json:"replace_first"`
	Mutations          []struct {
		Op, Path string
		Value    json.RawMessage
	}
}

func seedRecipeBytes(c seedReference, row seedRecipe) []byte {
	n := seedTree([]byte(c.DTOs[row.DTO].Canonical))
	for _, m := range row.Mutations {
		parts := strings.Split(m.Path, "/")[1:]
		for i := range parts {
			parts[i] = strings.ReplaceAll(strings.ReplaceAll(parts[i], "~1", "/"), "~0", "~")
		}
		target := n
		if m.Op == "append" {
			for _, part := range parts {
				_, target = target.child(part)
			}
			target.Children = append(target.Children, seedTree(m.Value))
			continue
		}
		for _, part := range parts[:len(parts)-1] {
			_, target = target.child(part)
		}
		key := parts[len(parts)-1]
		i, _ := target.child(key)
		switch m.Op {
		case "set":
			v := seedTree(m.Value)
			if i < 0 {
				target.Keys = append(target.Keys, key)
				target.Children = append(target.Children, v)
			} else {
				target.Children[i] = v
			}
		case "delete":
			target.Children = append(target.Children[:i], target.Children[i+1:]...)
			if target.Kind == '{' {
				target.Keys = append(target.Keys[:i], target.Keys[i+1:]...)
			}
		}
	}
	b := n.encode(false)
	switch row.Format {
	case "reversed_whitespace":
		var pretty bytes.Buffer
		json.Indent(&pretty, n.encode(true), "", " ")
		b = append([]byte(" \n"), pretty.Bytes()...)
		b = append(b, '\t', '\n')
	case "escaped_first_key":
		key := n.Keys[0]
		original, _ := json.Marshal(key)
		escaped := fmt.Sprintf("\"\\u%04x%s\"", key[0], key[1:])
		b = bytes.Replace(b, original, []byte(escaped), 1)
	case "duplicate_first_key", "duplicate_escaped_key":
		key := n.Keys[0]
		k, _ := json.Marshal(key)
		if row.Format == "duplicate_escaped_key" {
			k = []byte(fmt.Sprintf("\"\\u%04x%s\"", key[0], key[1:]))
		}
		b = b[:len(b)-1]
		b = append(b, ',')
		b = append(b, k...)
		b = append(b, ':')
		b = append(b, n.Children[0].encode(false)...)
		b = append(b, '}')
	case "pad_to_cap":
		b = append(b, bytes.Repeat([]byte{' '}, row.Bytes-len(b))...)
	}
	if len(row.Replace) > 0 {
		b = bytes.Replace(b, []byte(row.Replace[0]), []byte(row.Replace[1]), 1)
	}
	return b
}
func referenceSeedCodec(p Pin, kind string, raw []byte) ([]byte, error, bool) {
	switch kind {
	case "Pin":
		v, e := ParsePin(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalPin(v)
		return b, e, true
	case "Info":
		v, e := ParseInfo(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalInfo(p, v)
		return b, e, true
	case "Declaration":
		v, e := ParseDeclaration(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalDeclaration(p, v)
		return b, e, true
	case "SignedDeclaration":
		v, e := ParseSignedDeclaration(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalSignedDeclaration(p, v)
		return b, e, true
	case "Outpoint":
		v, e := ParseOutpoint(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalOutpoint(p, v)
		return b, e, true
	case "FundingCoin":
		v, e := ParseFundingCoin(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalFundingCoin(p, v)
		return b, e, true
	case "Cursor":
		v, e := ParseCursor(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalCursor(p, v)
		return b, e, true
	case "Tip":
		v, e := ParseTip(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalTip(p, v)
		return b, e, true
	case "Anchor":
		v, e := ParseAnchor(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalAnchor(p, v)
		return b, e, true
	case "Publication":
		v, e := ParsePublication(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalPublication(p, v)
		return b, e, true
	case "Target":
		v, e := ParseTarget(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalTarget(p, v)
		return b, e, true
	case "Finality":
		v, e := ParseFinality(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalFinality(p, v)
		return b, e, true
	case "Observation":
		v, e := ParseObservation(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalObservation(p, v)
		return b, e, true
	case "ScanRestart":
		v, e := ParseScanRestart(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalScanRestart(p, v)
		return b, e, true
	case "Conflict":
		v, e := ParseConflict(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalConflict(p, v)
		return b, e, true
	case "Record":
		v, e := ParseRecord(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalRecord(p, v)
		return b, e, true
	case "Status":
		v, e := ParseStatus(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalStatus(p, v)
		return b, e, true
	case "HistoryPage":
		v, e := ParseHistoryPage(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalHistoryPage(p, v)
		return b, e, true

	case "SignedCheckpoint":
		var v chaincfg.SignedCheckpoint
		if e := decode(raw, 4096, &v, false); e != nil {
			return nil, e, true
		}
		if !validCheckpoint(p, v) {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	case "Marker":
		var v marker
		if e := decode(raw, MaxMarkerSize, &v, false); e != nil {
			return nil, e, true
		}
		id, _ := ID(p)
		if !equal(v, marker{"slug-candidate-seeds", 1, id, p}) {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	case "Init":
		var v initIntent
		if e := decode(raw, MaxMetadataSize, &v, false); e != nil {
			return nil, e, true
		}
		id, _ := ID(p)
		if !equal(v, initIntent{1, id, p}) {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	case "Metadata":
		var v metadata
		if e := decode(raw, MaxMetadataSize, &v, false); e != nil {
			return nil, e, true
		}
		id, _ := ID(p)
		if !validMeta(v, p, id) {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	}
	return nil, nil, false
}
func TestSeedReferenceIdentitiesCalendarAndDTOs(t *testing.T) {
	requireNormal(t)
	c := readSeedReference(t)
	p := c.Pin.Value
	if RulesID() != c.Rules.ID {
		t.Fatal("rules")
	}
	id, e := ID(p)
	if e != nil || id != c.Pin.StoreID {
		t.Fatal("store", id, e)
	}
	derived, e := CandidatePin(p.SourcePin)
	if e != nil || !equal(derived, p) {
		t.Fatal("derived pin", e)
	}
	if len(c.Calendar.Rows) != 3861 {
		t.Fatal("calendar count")
	}
	for _, row := range c.Calendar.Rows {
		v, e := gymevent.EventForDate(p.SourcePin, row.Saturday)
		if e != nil || v != row.Event || v.CutoffMS != row.Cutoff || v.IncidentDeadlineMS != row.Deadline {
			t.Fatal(row.Saturday, e)
		}
		id, e := EventKey(p, v)
		if e != nil || id != row.EventKey {
			t.Fatal("event key", row.Saturday, e)
		}
	}
	for _, date := range c.Calendar.Invalid {
		if _, e := gymevent.EventForDate(p.SourcePin, date); e == nil {
			t.Fatal("accepted", date)
		}
	}
	count := 0
	for _, row := range c.DTOs {
		got, e, handled := referenceSeedCodec(p, row.Kind, row.Value)
		if !handled {
			continue
		}
		count++
		if e != nil || string(got) != row.Canonical {
			t.Errorf("%s/%s: %v canonical=%v", row.Kind, row.Name, e, string(got) == row.Canonical)
		}
	}
	if count != 44 {
		t.Errorf("core DTO coverage %d", count)
	}
}
func TestSeedReferenceClosedCodecRecipes(t *testing.T) {
	requireNormal(t)
	c := readSeedReference(t)
	count := 0
	for _, row := range c.Cases {
		if strings.HasPrefix(row.Kind, "Seed") {
			continue
		}
		t.Run(row.Kind+"/"+row.Name, func(t *testing.T) {
			b := seedRecipeBytes(c, row)
			h := sha256.Sum256(b)
			if len(b) != row.InputBytes || hex.EncodeToString(h[:]) != row.InputSHA {
				t.Fatal("recipe differs from independent pinned input", len(b), row.InputBytes)
			}
			got, e, handled := referenceSeedCodec(c.Pin.Value, row.Kind, b)
			if !handled {
				t.Fatal("missing core codec")
			}
			count++
			if (e == nil) != row.Accepted {
				t.Fatalf("accepted=%v want=%v: %v", e == nil, row.Accepted, e)
			}
			if e == nil && (row.Expected == nil || string(got) != c.DTOs[*row.Expected].Canonical) {
				t.Fatal("canonical differs")
			}
		})
	}
	t.Log("independent core codec cases", count)
}
func TestSeedReferenceScanFoldsAndModeledStatuses(t *testing.T) {
	requireNormal(t)
	c := readSeedReference(t)
	p := c.Pin.Value
	for _, row := range c.Scans {
		v := initialCursor(p, row.EventKey, row.Generation)
		if v.ScanDigest != row.Initial {
			t.Fatal("initial fold", row.Kind, row.Generation)
		}
		for _, step := range row.Steps {
			advanceCursor(&v, step.Height, step.Hash, row.Kind == "event")
			if v.ScanDigest != step.Digest {
				t.Fatal("step fold", row.Kind, row.Generation, step.Height)
			}
		}
	}
	records := map[string]Record{}
	for i, row := range c.Records {
		r, e := ParseRecord(p, []byte(row.Canonical))
		if e != nil {
			t.Errorf("record state %d: %v", i, e)
			continue
		}
		d, e := RecordDigest(p, r)
		if e != nil || d != row.Digest {
			t.Errorf("record digest %d: %v", i, e)
		}
		records[row.Digest] = r
	}
	// These compare only pure projections of independently modeled durable states;
	// they do not claim to execute modeled network or process-crash actions.
	count := 0
	for _, scenario := range c.Scenarios {
		for i, step := range scenario.Steps {
			if step.Status == nil {
				continue
			}
			var got Status
			if step.Expected.Digest == "" {
				got = absentStatus(p, step.Status.Event, step.Status.ObservedAtMS)
			} else {
				r, ok := records[step.Expected.Digest]
				if !ok {
					continue
				}
				got = statusFor(p, r, step.Status.ObservedAtMS, step.Expected.FundingState, step.Expected.FundingGeneration)
			}
			count++
			if !equal(got, *step.Status) {
				t.Errorf("modeled %s/%d/%s status differs", scenario.Name, i, step.Operation)
			}
		}
	}
	t.Log("modeled status projections", count)
}

func TestSeedReferenceFullCarrierBodies(t *testing.T) {
	requireNormal(t)
	c := readSeedReference(t)
	raw, e := os.ReadFile("testdata/reference/fixtures/block_index.json")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "2614db26e86aa553d51659981c295fbcb9d83a8d7681ec4ac28bbea14430ab69" {
		t.Fatal("block index changed")
	}
	var index struct {
		Anchor  Anchor
		Maximum Anchor `json:"maximum_anchor"`
		Blocks  []struct {
			File, Hash, SHA256 string
			Bytes              int
			Height             uint32
		}
	}
	if e = json.Unmarshal(raw, &index); e != nil {
		t.Fatal(e)
	}
	// This tests full body extraction/first tx ordering. Normal Argon and full
	// validating-node admission are explicitly separate adapter/integration gates.
	for _, want := range []Anchor{index.Anchor, index.Maximum} {
		found := false
		for _, b := range index.Blocks {
			if b.Hash != want.Hash {
				continue
			}
			found = true
			body, e := os.ReadFile(filepath.Join("testdata/reference", b.File))
			if e != nil || len(body) != b.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != b.SHA256 {
				t.Fatal("body pin", b.File, e)
			}
			var block wire.MsgBlock
			reader := bytes.NewReader(body)
			if block.Deserialize(reader) != nil || reader.Len() != 0 {
				t.Fatal("wire body")
			}
			var payload [73]byte
			copy(payload[:], rawID(want.Payload))
			got, e := firstAnchor(context.Background(), &block, b.Height, b.Hash, payload)
			if e != nil || got == nil || !equal(*got, want) {
				t.Fatal("full carrier", b.File, e)
			}
			if e = validateAnchor(c.Pin.Value, *got); e != nil {
				t.Fatal(e)
			}
		}
		if !found {
			t.Fatal("missing full anchor body")
		}
	}
}
