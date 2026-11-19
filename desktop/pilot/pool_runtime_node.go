// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
)

// Only the separate shared pool namespace selects transaction indexing. The
// original solo helper keeps startNode's unchanged false profile.
func (r *PoolRuntime) openPoolNodes(ctx context.Context) (*pilotNodes, error) {
	ports := make([]uint16, 4)
	seen := map[uint16]bool{r.marker.PoolPort: true, r.marker.AdmissionPort: true}
	for i := range ports {
		for attempts := 0; attempts < 16; attempts++ {
			port, e := unusedPort()
			if e != nil {
				return nil, e
			}
			if !seen[port] {
				ports[i] = port
				seen[port] = true
				break
			}
		}
		if ports[i] == 0 {
			return nil, ErrPoolRuntime
		}
	}
	peer, e := startNodeProfile(r.config.NodeBinary, r.config.DataDir, "peer", ports[0], ports[1], ports[3], true, false)
	if e != nil {
		return nil, e
	}
	primary, e := startNodeProfile(r.config.NodeBinary, r.config.DataDir, "primary", ports[2], 0, ports[1], false, true)
	if e != nil {
		return nil, errors.Join(e, peer.stop())
	}
	nodes := &pilotNodes{primary: primary, peer: peer}
	if e = waitNode(ctx, primary, true); e == nil {
		e = waitNode(ctx, peer, false)
	}
	if e != nil {
		return nil, errors.Join(e, nodes.Close())
	}
	return nodes, nil
}
