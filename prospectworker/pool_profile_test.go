//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPoolCheapProfileRefusesBeforeState(t *testing.T) {
	require := func(err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "slugtestpow") {
			t.Fatal("production profile was not checked first", err)
		}
	}
	c := PoolConfig{} // Deliberately invalid: profile refusal precedes parsing.
	path := filepath.Join(t.TempDir(), "absent")
	box, err := CreatePoolOutbox(path, c)
	require(err)
	if box != nil {
		t.Fatal("cheap-profile constructor leaked a journal")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cheap-profile Create touched path")
	}
	box, err = OpenPoolOutbox(path, c)
	require(err)
	if box != nil {
		t.Fatal("cheap-profile Open leaked a journal")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cheap-profile Open touched absent path")
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := []byte("untouched journal marker")
	marker := filepath.Join(path, poolMarkerName)
	if err = os.WriteFile(marker, sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	box, err = OpenPoolOutbox(path, c)
	require(err)
	if box != nil {
		t.Fatal("cheap-profile Open accepted marker")
	}
	after, err := os.ReadFile(marker)
	if err != nil || !bytes.Equal(after, sentinel) {
		t.Fatal("cheap-profile Open altered marker", err)
	}
	w, err := NewPool(c, nil, nil, nil, nil, nil)
	require(err)
	if w != nil {
		t.Fatal("cheap-profile constructor leaked a worker")
	}
	w, err = NewPoolWithHooks(c, nil, nil, nil, nil, nil, nil, nil)
	require(err)
	if w != nil {
		t.Fatal("cheap-profile hook constructor leaked a worker")
	}
	require((&PoolWorker{}).Run(context.Background()))
	rows, err := (&PoolOutbox{}).List(0, 1)
	require(err)
	if rows != nil {
		t.Fatal("cheap profile disclosed journal rows")
	}
}
