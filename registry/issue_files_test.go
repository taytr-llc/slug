// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoverableIssuerFailedFileCloseRetainsOriginal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "closed-")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("conditional preceding read failure")
	s := new(ManifestStore)
	got := s.closeIssueFile(f, cause)
	if !errors.Is(got, cause) || !errors.Is(got, os.ErrClosed) || s.failedIssueFile != f {
		t.Fatal("exact original failed descriptor", got)
	}
	retained, err := failedIssueFileOwner(s, got)
	if retained != s || err != got {
		t.Fatal("failed owner lost", err)
	}
	// This is a conditional helper test using an already-closed descriptor, not
	// a claim to have caused a kernel Close failure during a constructor.
	for i := 0; i < 2; i++ {
		if err := s.Close(); err != got {
			t.Fatal("sticky close changed or retried", err)
		}
	}
	if s.failedIssueFile != f {
		t.Fatal("descriptor replaced")
	}
}
func TestRecoverableIssuerFileSuccessAndFailureOwnership(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "marker")
	s := new(ManifestStore)
	value := struct {
		Version uint32 `json:"version"`
		Name    string `json:"name"`
	}{1, "public"}
	if err := s.writeIssueFile(path, value); err != nil {
		t.Fatal(err)
	}
	raw, err := s.readIssueFile(path, maxIssueMetadataSize)
	want, _ := json.Marshal(value)
	want = append(want, '\n')
	if err != nil || !bytes.Equal(raw, want) || s.failedIssueFile != nil {
		t.Fatal("unchanged canonical file", err)
	}
	if err = s.writeIssueFile(path, value); !os.IsExist(err) {
		t.Fatal("exclusive file", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("conditional read failure with successful close")
	if err = s.closeIssueFile(f, cause); err != cause {
		t.Fatal(err)
	}
	if retained, err := failedIssueFileOwner(s, cause); retained != nil || err != cause {
		t.Fatal("successful close retained an owner", err)
	}
	if _, err = s.readIssueFile(path, len(want)-1); err != ErrCorrupt {
		t.Fatal("bounded read", err)
	}
	if s.failedIssueFile != nil {
		t.Fatal("successful close marked failed")
	}
}
