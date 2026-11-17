// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectjob

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/fighterdescriptor"
)

func manifestTestJob(t testing.TB) (ManifestJob, Job) {
	t.Helper()
	raw, err := os.ReadFile("testdata/jobs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Job Job `json:"job"`
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) < 1 {
		t.Fatal("old independent inputs", err)
	}
	old := fixtures[0].Job
	id, err := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	job, err := NewManifest(old.Context, strings.Repeat("01", 32), id, old.PublicKey, old.Challenge, old.IssuedAtMS)
	if err != nil {
		t.Fatal(err)
	}
	return job, old
}

func TestManifestNormalizationAndTime(t *testing.T) {
	job, old := manifestTestJob(t)
	context := job.Context
	context.Genesis = strings.ToUpper(context.Genesis)
	context.ProspectTarget = strings.ToUpper(context.ProspectTarget)
	context.ParentHash = strings.ToUpper(context.ParentHash)
	got, err := NewManifest(context, strings.ToUpper(job.AuthorityID), strings.ToUpper(job.DescriptorID), strings.ToUpper(job.PublicKey), strings.ToUpper(job.Challenge), job.IssuedAtMS)
	if err != nil || got != job {
		t.Fatal("normalization changed job", err)
	}
	if len(manifestPreimage(job)) != 328 {
		t.Fatal("preimage width changed")
	}
	for _, issued := range []int64{1, math.MaxInt64 - LifetimeMS} {
		got, err := NewManifest(job.Context, job.AuthorityID, job.DescriptorID, job.PublicKey, job.Challenge, issued)
		if err != nil || got.ExpiresAtMS-issued != LifetimeMS || ValidateManifest(got) != nil {
			t.Fatal("valid issuance endpoint failed", err)
		}
	}
	for _, issued := range []int64{0, -1, math.MinInt64, math.MaxInt64 - LifetimeMS + 1, math.MaxInt64} {
		got, err := NewManifest(job.Context, job.AuthorityID, job.DescriptorID, job.PublicKey, job.Challenge, issued)
		if err == nil || got != (ManifestJob{}) {
			t.Fatal("bad time returned partial job")
		}
	}
	zero, err := NewManifest(job.Context, strings.Repeat("0", 64), job.DescriptorID, job.PublicKey, strings.Repeat("0", 64), 1)
	if err != nil || ValidateManifest(zero) != nil {
		t.Fatal("zero byte values rejected", err)
	}
	other, err := NewManifest(job.Context, strings.Repeat("02", 32), job.DescriptorID, job.PublicKey, job.Challenge, job.IssuedAtMS)
	if err != nil || other.ID == job.ID {
		t.Fatal("authority not bound", err)
	}
	a, _ := ManifestCommitmentScript(job)
	b, _ := ManifestCommitmentScript(other)
	if bytes.Equal(a, b) {
		t.Fatal("authority did not change work commitment")
	}
	oldAgain, err := New(old.Context, old.PublicKey, old.Challenge, old.IssuedAtMS)
	if err != nil || oldAgain != old || old.ID == job.ID {
		t.Fatal("v1 parity/domain changed", err)
	}
	for _, bad := range []string{"", strings.Repeat("0", 63), strings.Repeat("0", 66), strings.Repeat("g", 64)} {
		if got, err := NewManifest(job.Context, bad, job.DescriptorID, job.PublicKey, job.Challenge, 1); err == nil || got != (ManifestJob{}) {
			t.Fatal("malformed authority accepted")
		}
	}
}

func TestManifestWholeValueAndSupport(t *testing.T) {
	base, _ := manifestTestJob(t)
	var visit func(reflect.Value, string)
	visit = func(v reflect.Value, prefix string) {
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if v.Field(i).Kind() == reflect.Struct {
				visit(v.Field(i), prefix+field.Name+".")
				continue
			}
			name := prefix + field.Name
			t.Run(name, func(t *testing.T) {
				job := base
				value := reflect.ValueOf(&job).Elem()
				for _, part := range strings.Split(name, ".") {
					value = value.FieldByName(part)
				}
				switch value.Kind() {
				case reflect.String:
					value.SetString(value.String() + "0")
				case reflect.Uint32:
					value.SetUint(value.Uint() + 1)
				case reflect.Int64:
					value.SetInt(value.Int() + 1)
				}
				if ValidateManifest(job) == nil {
					t.Fatal("changed immutable field accepted")
				}
				if out, err := MarshalManifestJob(job); err == nil || out != nil {
					t.Fatal("invalid serialization succeeded")
				}
				if out, err := ManifestCommitmentScript(job); err == nil || out != nil {
					t.Fatal("invalid commitment succeeded")
				}
			})
		}
	}
	visit(reflect.ValueOf(base), "")
	unknown := fighterdescriptor.CandidateDescriptor()
	unknown.RendererVersion++
	id, err := fighterdescriptor.ID(unknown)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := NewManifest(base.Context, base.AuthorityID, id, base.PublicKey, base.Challenge, base.IssuedAtMS); err == nil || got != (ManifestJob{}) {
		t.Fatal("unsupported descriptor operational")
	}
	bad := base
	bad.DescriptorID = id
	bad.ID = strings.Repeat("0", 64)
	if ValidateManifest(bad) == nil {
		t.Fatal("unsupported descriptor validated")
	}
	bad = base
	bad.Context.DNAVersion = 2
	if got, err := NewManifest(bad.Context, bad.AuthorityID, bad.DescriptorID, bad.PublicKey, bad.Challenge, bad.IssuedAtMS); err == nil || got != (ManifestJob{}) {
		t.Fatal("DNA link mismatch accepted")
	}
}

func TestManifestTransportAndBounds(t *testing.T) {
	job, old := manifestTestJob(t)
	raw, err := MarshalManifestJob(job)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		t.Fatal("marshal invalid JSON")
	}
	reordered, _ := json.Marshal(object)
	pretty, _ := json.MarshalIndent(job, "", "  ")
	escaped := bytes.Replace(raw, []byte(`"version"`), []byte(`"ver\u0073ion"`), 1)
	escaped = bytes.Replace(escaped, []byte(`"authority_id":"01`), []byte(`"authority_id":"\u00301`), 1)
	for _, valid := range [][]byte{raw, reordered, pretty, escaped, append(append([]byte("\n\t "), raw...), []byte(" \r\n")...), append(bytes.Clone(raw), bytes.Repeat([]byte(" "), MaxManifestJobSize-len(raw))...)} {
		got, err := ParseManifestJob(valid)
		if err != nil || got != job {
			t.Fatal("transport spelling changed value", err)
		}
		canonical, err := MarshalManifestJob(got)
		if err != nil || !bytes.Equal(canonical, raw) {
			t.Fatal("canonical output changed", err)
		}
	}
	for key, value := range object {
		t.Run(key, func(t *testing.T) {
			prefix := []byte(`"` + key + `":`)
			needle := append(bytes.Clone(prefix), value...)
			for _, replacement := range []string{"null", "true", "[]"} {
				bad := bytes.Replace(raw, needle, append(bytes.Clone(prefix), replacement...), 1)
				if got, err := ParseManifestJob(bad); err == nil || got != (ManifestJob{}) {
					t.Fatal("bad scalar accepted")
				}
			}
			dup := bytes.Replace(raw, needle, append(append(bytes.Clone(needle), ','), needle...), 1)
			if got, err := ParseManifestJob(dup); err == nil || got != (ManifestJob{}) {
				t.Fatal("duplicate accepted")
			}
			alias := bytes.Replace(raw, prefix, []byte(`"`+strings.ToUpper(key)+`":`), 1)
			if got, err := ParseManifestJob(alias); err == nil || got != (ManifestJob{}) {
				t.Fatal("case alias accepted")
			}
			missing := bytes.Replace(raw, append(bytes.Clone(needle), ','), nil, 1)
			if bytes.Equal(missing, raw) {
				missing = bytes.Replace(raw, append([]byte(","), needle...), nil, 1)
			}
			if got, err := ParseManifestJob(missing); err == nil || got != (ManifestJob{}) {
				t.Fatal("missing field accepted")
			}
		})
	}
	for _, value := range []string{`"2"`, `2.0`, `2e0`, `-2`, `4294967296`} {
		bad := bytes.Replace(raw, []byte(`"version":2`), []byte(`"version":`+value), 1)
		if got, err := ParseManifestJob(bad); err == nil || got != (ManifestJob{}) {
			t.Fatal("bad version number accepted")
		}
	}
	context := object["context"]
	for _, length := range []int{4096, 4097} {
		padded := append(bytes.Clone(context[:len(context)-1]), bytes.Repeat([]byte(" "), length-len(context))...)
		padded = append(padded, '}')
		input := bytes.Replace(raw, context, padded, 1)
		got, err := ParseManifestJob(input)
		if length == 4096 {
			if err != nil || got != job {
				t.Fatal("exact context bound rejected", err)
			}
		} else if err == nil || got != (ManifestJob{}) {
			t.Fatal("nested context bound bypass")
		}
	}
	for _, bad := range [][]byte{nil, []byte("null"), []byte("[]"), append(bytes.Clone(raw), 'x'), append(bytes.Clone(raw), raw...), append(bytes.Clone(raw), bytes.Repeat([]byte(" "), MaxManifestJobSize-len(raw)+1)...), bytes.Replace(raw, []byte(`"version":2`), []byte(`"version":2,"ver\u0073ion":2`), 1), []byte(strings.Repeat(`{"a":`, 10) + "1" + strings.Repeat("}", 10))} {
		if got, err := ParseManifestJob(bad); err == nil || got != (ManifestJob{}) {
			t.Fatal("bad document accepted")
		}
	}
	if got, err := ParseJob(raw); err == nil || got != (Job{}) {
		t.Fatal("old reader accepted new job")
	}
	oldJSON, _ := json.Marshal(old)
	if got, err := ParseManifestJob(oldJSON); err == nil || got != (ManifestJob{}) {
		t.Fatal("new reader downgraded")
	}
}

func TestManifestConcurrentOwnership(t *testing.T) {
	job, _ := manifestTestJob(t)
	before := job
	raw, _ := MarshalManifestJob(job)
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				got, err := ParseManifestJob(raw)
				if err != nil || got != job {
					t.Error("parse changed", err)
					return
				}
				copy, err := MarshalManifestJob(got)
				if err != nil || !bytes.Equal(copy, raw) {
					t.Error("marshal changed", err)
					return
				}
				copy[0] = 'X'
				if ValidateManifest(job) != nil {
					t.Error("validation changed")
				}
			}
		}()
	}
	wg.Wait()
	if job != before {
		t.Fatal("input mutated")
	}
}

func FuzzManifestJobParse(f *testing.F) {
	job, _ := manifestTestJob(f)
	raw, _ := MarshalManifestJob(job)
	f.Add(raw)
	f.Add([]byte("{}"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		got, err := ParseManifestJob(raw)
		if err != nil {
			if got != (ManifestJob{}) {
				t.Fatal("partial result")
			}
			return
		}
		canonical, err := MarshalManifestJob(got)
		if err != nil {
			t.Fatal("valid result not serializable")
		}
		again, err := ParseManifestJob(canonical)
		if err != nil || again != got {
			t.Fatal("round trip changed value")
		}
	})
}
