// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.
package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeExportAtomicAndSymlinkRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipt.json")
	raw := []byte("{\"public\":true}\n")
	if err := saveReceipt(path, raw); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(raw) {
		t.Fatal("wrong export")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("receipt permissions")
	}
	link := filepath.Join(t.TempDir(), "link.json")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if saveReceipt(link, []byte("replacement")) == nil {
		t.Fatal("followed symbolic link")
	}
	got, _ = os.ReadFile(path)
	if string(got) != string(raw) {
		t.Fatal("modified linked target")
	}
	if saveReceipt(filepath.Dir(path), raw) == nil {
		t.Fatal("replaced directory")
	}
}
func TestEmbeddedAssetSecurity(t *testing.T) {
	handler := assetSecurity(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/", nil))
	if recorder.Header().Get("Content-Security-Policy") != contentPolicy || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing asset policy")
	}
}

func TestReceiptCannotReplaceManagedDataThroughParentAlias(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "pilot")
	if err := os.Mkdir(managed, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("retained identity")
	target := filepath.Join(managed, "SLUG-PILOT.json")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(managed, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{target, filepath.Join(alias, "SLUG-PILOT.json"), filepath.Join(managed, "new.json")} {
		if saveReceipt(path, []byte("receipt"), managed) == nil {
			t.Fatal("accepted managed destination")
		}
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != string(original) {
		t.Fatal("changed managed data")
	}
	if err := saveReceipt(filepath.Join(root, "receipt.json"), []byte("public receipt"), managed); err != nil {
		t.Fatal(err)
	}
}
