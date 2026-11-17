// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
)

func (c *ManifestController) openNodes(ctx context.Context) (*pilotNodes, error) {
	ports := make([]uint16, 4)
	seen := map[uint16]bool{c.manifest.AuthorityPort: true}
	for i := range ports {
		for {
			port, err := unusedPort()
			if err != nil {
				return nil, errors.New("loopback unavailable")
			}
			if !seen[port] {
				ports[i], seen[port] = port, true
				break
			}
		}
	}
	peer, err := startNode(c.config.NodeBinary, c.config.DataDir, "peer", ports[0], ports[1], ports[3], true)
	if err != nil {
		return nil, err
	}
	primary, err := startNode(c.config.NodeBinary, c.config.DataDir, "primary", ports[2], 0, ports[1], false)
	if err != nil {
		if closeErr := peer.stop(); closeErr != nil {
			c.noteShutdown("pilot node startup cleanup failed")
			return nil, errors.Join(err, errPilotNodeClose)
		}
		return nil, err
	}
	nodes := &pilotNodes{primary: primary, peer: peer}
	cleanup := func(err error) error {
		if nodes.Close() != nil {
			c.noteShutdown("pilot node startup cleanup failed")
			return errors.Join(err, errPilotNodeClose)
		}
		return err
	}
	if err := waitNode(ctx, primary, true); err != nil {
		return nil, cleanup(err)
	}
	if err := waitNode(ctx, peer, false); err != nil {
		return nil, cleanup(err)
	}
	return nodes, nil
}
