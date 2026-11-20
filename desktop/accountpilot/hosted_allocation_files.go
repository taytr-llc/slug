// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"reflect"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/desktop/pilot"
)

const hostedAllocationGuardName = "SLUG-HOSTED-ALLOCATION-V1.json"

type hostedInventoryPin struct {
	Fixture     string `json:"fixture"`
	InventoryID string `json:"inventory_id"`
}
type hostedAllocationGuard struct {
	Application string               `json:"application"`
	Version     uint32               `json:"version"`
	Mode        string               `json:"mode"`
	HostedInit  HostedInit           `json:"hosted_init"`
	Inventories []hostedInventoryPin `json:"inventories"`
}
type hostedAllocationOwner struct {
	guard          hostedAllocationGuard
	setup          *pilot.HostedAllocationSetup
	missing        map[string]bool
	archive        *pilot.PoolRecoveryArchive
	recovery       *pilot.HostedRecoveryOwner
	failedRecovery *pilot.HostedRecoveryOwner
}

func validHostedAllocationGuard(g hostedAllocationGuard) bool {
	return g.Application == "slug-hosted-allocation-owner" && g.Version == 1 && g.Mode == pilot.HostedRecoveryPoolMode && validHostedInit(g.HostedInit) && len(g.Inventories) == 2 && g.Inventories[0].Fixture == "zero256" && g.Inventories[1].Fixture == "incrementing256" && hostedID(g.Inventories[0].InventoryID) && hostedID(g.Inventories[1].InventoryID) && g.Inventories[0].InventoryID != g.Inventories[1].InventoryID
}
func readHostedAllocationGuard(root string) (hostedAllocationGuard, bool, error) {
	var g hostedAllocationGuard
	found, e := readHostedControl(filepath.Join(root, hostedAllocationGuardName), 32768, &g)
	if e == nil && found && !validHostedAllocationGuard(g) {
		e = pilot.ErrManifestInitialization
	}
	return g, found, e
}
func openHostedAllocationRoot(root string, options pilot.PoolOptions) (*leveldb.DB, error) {
	options, e := pilot.ResolvePoolOptions(options)
	if e != nil {
		return nil, e
	}
	if e = hostedRootPathMode(root, true); e != nil {
		return nil, e
	}
	if e = os.Mkdir(root, 0700); e != nil && !os.IsExist(e) {
		return nil, e
	}
	check := func() (hostedAllocationGuard, bool, error) {
		g, found, e := readHostedAllocationGuard(root)
		if e != nil {
			return g, found, e
		}
		if !found {
			entries, e := hostedDirectoryEntries(root, 1)
			if e != nil {
				return g, false, e
			}
			for _, v := range entries {
				if v.Name() != "instance-lock" || !v.IsDir() {
					return g, false, pilot.ErrManifestInitialization
				}
			}
			return g, false, nil
		}
		if g.HostedInit.DescriptorID != options.CandidateDescriptorID {
			return g, true, pilot.ErrManifestInitialization
		}
		init, has, e := readHostedInit(root)
		if e != nil {
			return g, true, e
		}
		if has {
			if !reflect.DeepEqual(init, g.HostedInit) {
				return g, true, pilot.ErrManifestInitialization
			}
			_, e = readHostedSetupPrefixMode(root, init, true)
			return g, true, e
		}
		entries, e := hostedDirectoryEntries(root, 2)
		if e != nil {
			return g, true, e
		}
		for _, v := range entries {
			if v.Name() != hostedAllocationGuardName && !(v.Name() == "instance-lock" && v.IsDir()) {
				return g, true, pilot.ErrManifestInitialization
			}
		}
		return g, true, nil
	}
	before, found, e := check()
	if e != nil {
		return nil, e
	}
	lock, e := leveldb.OpenFile(filepath.Join(root, "instance-lock"), nil)
	if e != nil {
		return nil, pilot.ErrManifestInitialization
	}
	after, still, e := check()
	if e != nil || found != still || !reflect.DeepEqual(before, after) {
		return nil, errors.Join(pilot.ErrManifestInitialization, lock.Close())
	}
	return lock, nil
}
func (c *PoolController) prepareHostedAllocationGuard() error {
	h := c.hosted
	g, found, e := readHostedAllocationGuard(c.config.DataDir)
	if e != nil {
		return e
	}
	if !found {
		entries, e := hostedDirectoryEntries(c.config.DataDir, 1)
		if e != nil {
			return e
		}
		for _, v := range entries {
			if v.Name() != "instance-lock" || !v.IsDir() {
				return pilot.ErrManifestInitialization
			}
		}
		g = hostedAllocationGuard{Application: "slug-hosted-allocation-owner", Version: 1, Mode: pilot.HostedRecoveryPoolMode, HostedInit: h.init}
		for _, f := range hostedFixtures {
			id, e := hostedRandomID(rand.Reader)
			if e != nil {
				return e
			}
			g.Inventories = append(g.Inventories, hostedInventoryPin{f, id})
		}
		if !validHostedAllocationGuard(g) {
			return pilot.ErrManifestInitialization
		}
		if e = writeHostedControl(filepath.Join(c.config.DataDir, hostedAllocationGuardName), g, 32768); e != nil {
			return e
		}
		parent, e := os.Open(filepath.Dir(c.config.DataDir))
		if e != nil {
			return e
		}
		if e = errors.Join(parent.Sync(), parent.Close()); e != nil {
			return e
		}
	}
	if !reflect.DeepEqual(g.HostedInit, h.init) {
		return pilot.ErrManifestInitialization
	}
	h.allocation.guard = g
	return nil
}
