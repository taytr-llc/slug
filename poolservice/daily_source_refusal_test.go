// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"testing"

	"github.com/taytr-llc/slug/poolshare"
)

func TestPoolServiceDoesNotAdoptDailySource(t *testing.T) {
	c, _, _ := clientFixture(t)
	if _, err := BindingID(c.Source); err != nil {
		t.Fatal("original V1 source", err)
	}
	c.Source.Version = poolshare.Version2
	if id, err := BindingID(c.Source); err == nil || id != "" {
		t.Fatal("legacy binding reinterpreted daily source")
	}
	if _, err := ValidateClientConfig(c); err == nil {
		t.Fatal("legacy client adopted daily source")
	}
	c.Source.MaxWorks, c.Source.MaxSubmissions, c.Source.MaxBytes = 8192, 16384, 8<<30
	if id, err := BindingID(c.Source); err == nil || id != "" {
		t.Fatal("expanded daily source entered V1 binding")
	}
	if _, err := ValidateClientConfig(c); err == nil {
		t.Fatal("expanded daily source entered V1 client")
	}
}
