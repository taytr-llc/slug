// Copyright (c) 2026 The SLUG developers. ISC license.
package recoveryfile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

func validIdentity(i Identity) bool {
	for _, s := range []string{i.Device, i.Inode} {
		if len(s) != 16 || strings.ToLower(s) != s {
			return false
		}
		if _, e := hex.DecodeString(s); e != nil {
			return false
		}
	}
	return i.Inode != "0000000000000000"
}
func validPath(path string) bool {
	return len(path) > 0 && len(path) <= MaxPath && filepath.IsAbs(path) && filepath.Clean(path) == path && filepath.Base(path) != "." && filepath.Base(path) != string(filepath.Separator) && !strings.ContainsAny(path, "\x00\r\n\t") && len(filepath.Base(path)) <= 255
}
func ValidatePin(p Pin, mode string) error {
	if (mode != CreateMode && mode != ReadMode) || p.Version != 1 || p.Mode != mode || !validPath(p.Path) || !validIdentity(p.Parent) {
		return ErrInvalid
	}
	if mode == CreateMode {
		if p.File != nil {
			return ErrInvalid
		}
		return nil
	}
	if p.File == nil || !validIdentity(Identity{p.File.Device, p.File.Inode}) || p.File.Size < 1 || p.File.Size > MaxSize || len(p.File.ModifiedNS) > 32 {
		return ErrInvalid
	}
	n, ok := new(big.Int).SetString(p.File.ModifiedNS, 10)
	if !ok || n.String() != p.File.ModifiedNS {
		return ErrInvalid
	}
	return nil
}
func within(root, path string) bool {
	r, e := filepath.Rel(root, path)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}

// Capture resolves legitimate selected parent aliases once, then walks only
// that canonical path without following substitutions. No file bytes are read.
func Capture(path string, protected []string, mode string) (*Selection, error) {
	if !supportedPlatform() {
		return nil, ErrUnsupported
	}
	if !validPath(path) || (mode != CreateMode && mode != ReadMode) {
		return nil, ErrInvalid
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(path))
	if e != nil {
		return nil, ErrInvalid
	}
	path = filepath.Join(parent, filepath.Base(path))
	if !validPath(path) {
		return nil, ErrInvalid
	}
	dir, stamp, e := openCheckedParent(path, protected)
	if e != nil {
		return nil, e
	}
	p := Pin{Version: 1, Mode: mode, Path: path, Parent: stamp.identity}
	s := &Selection{pin: p, dir: dir}
	if mode == CreateMode {
		if _, e = inspectRelative(dir, filepath.Base(path)); !os.IsNotExist(e) {
			if e == nil {
				return nil, closeOwned(ErrConflict, s)
			}
			return nil, closeOwned(ErrInvalid, s)
		}
	} else {
		s.file, e = openReadRelative(dir, filepath.Base(path))
		if e != nil {
			return nil, closeOwned(ErrInvalid, s)
		}
		st, e := stampFile(s.file)
		if e != nil || !st.regular || st.size < 1 || st.size > MaxSize {
			return nil, closeOwned(ErrInvalid, s)
		}
		f := st.fileIdentity()
		s.pin.File = &f
	}
	if ValidatePin(s.pin, mode) != nil {
		return nil, closeOwned(ErrInvalid, s)
	}
	return s, nil
}

// Open verifies the native-held identity before any bytes or temporary files
// are created. It never resolves aliases in a Pin again.
func Open(pin Pin, protected []string) (*Handle, error) {
	if !supportedPlatform() {
		return nil, ErrUnsupported
	}
	if ValidatePin(pin, pin.Mode) != nil {
		return nil, ErrInvalid
	}
	dir, st, e := openCheckedParent(pin.Path, protected)
	if e != nil {
		return nil, e
	}
	h := &Handle{pin: ownedPin(pin), dir: dir, name: filepath.Base(pin.Path), ops: defaultOperations()}
	fail := func(e error) (*Handle, error) { return nil, closeOwned(e, h) }
	if st.identity != pin.Parent {
		return fail(ErrInvalid)
	}
	if pin.Mode == CreateMode {
		if _, e = inspectRelative(dir, h.name); !os.IsNotExist(e) {
			if e == nil {
				return fail(ErrConflict)
			}
			return fail(ErrInvalid)
		}
		var nonce [24]byte
		if _, e = rand.Read(nonce[:]); e != nil {
			return fail(ErrInvalid)
		}
		h.temporary = ".slug-recovery-" + hex.EncodeToString(nonce[:])
	} else {
		h.file, e = openReadRelative(dir, h.name)
		if e != nil {
			return fail(ErrInvalid)
		}
		st, e = stampFile(h.file)
		if e != nil || !st.regular || st.fileIdentity() != *pin.File {
			return fail(ErrInvalid)
		}
	}
	return h, nil
}

func openCheckedParent(path string, protected []string) (*os.File, fileStamp, error) {
	if len(protected) > 64 {
		return nil, fileStamp{}, ErrInvalid
	}
	parent := filepath.Dir(path)
	dir, ancestors, e := walkDirectory(parent, true)
	if e != nil {
		return nil, fileStamp{}, errors.Join(ErrInvalid, e)
	}
	fail := func(cause error) (*os.File, fileStamp, error) {
		return nil, fileStamp{}, closeOwned(cause, dir)
	}
	for _, root := range protected {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, "\x00\r\n\t") {
			return fail(ErrInvalid)
		}
		resolved, e := filepath.EvalSymlinks(root)
		if e != nil {
			if !os.IsNotExist(e) {
				return fail(ErrInvalid)
			}
			resolved = root
		}
		if within(resolved, path) {
			return fail(ErrInvalid)
		}
		protectedDir, _, e := walkDirectory(resolved, false)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return fail(errors.Join(ErrInvalid, e))
		}
		protectedStamp, se := stampFile(protectedDir)
		ce := closeOwned(se, protectedDir)
		if ce != nil {
			return fail(errors.Join(ErrInvalid, ce))
		}
		for _, id := range ancestors {
			if id == protectedStamp.identity {
				return fail(ErrInvalid)
			}
		}
	}
	st, e := stampFile(dir)
	if e != nil || !st.directory {
		return fail(ErrInvalid)
	}
	return dir, st, nil
}
