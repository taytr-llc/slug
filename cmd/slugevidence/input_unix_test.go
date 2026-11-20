//go:build darwin || linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestEvidenceCLIRejectsSpecialInputsWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "swapped-input")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "input-alias")
	if err := os.Symlink(fifo, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fifo, alias, "/dev/null", dir} {
		done := make(chan error, 1)
		go func() { _, err := readBounded(path, 100); done <- err }()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("special input accepted")
			}
		case <-time.After(time.Second):
			// A blocking regression must be joined before failure. Opening the
			// owned FIFO read/write releases an accidentally blocked reader.
			f, e := os.OpenFile(fifo, os.O_RDWR|unix.O_NONBLOCK, 0)
			if e == nil {
				defer f.Close()
			}
			<-done
			t.Fatal("special input open blocked")
		}
	}
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte("public"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(regular, alias); err != nil {
		t.Fatal(err)
	}
	if b, e := readBounded(alias, 6); e != nil || string(b) != "public" {
		t.Fatal("legitimate regular-file alias", e)
	}
}
