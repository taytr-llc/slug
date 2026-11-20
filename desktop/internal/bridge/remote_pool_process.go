// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/taytr-llc/slug/desktop/pilot"
)

// Only paths to independently selected protected inputs enter argv. The bridge
// never reads RPC credentials, enrollment bearers, wallet secrets or key files.
type RemotePoolLaunchSelection struct {
	ExistingRoot, AttachmentRoot                string
	ConfigPath, NodeRPCPath                     string
	PoolCredentialPath, AdmissionCredentialPath string
	Options                                     pilot.PoolOptions
}

func remotePoolLaunchCommand(helper string, s RemotePoolLaunchSelection) (*exec.Cmd, error) {
	options, e := pilot.ResolvePoolOptions(s.Options)
	if e != nil {
		return nil, ErrUnavailable
	}
	for _, p := range []string{helper, s.ExistingRoot, s.AttachmentRoot, s.ConfigPath, s.NodeRPCPath, s.PoolCredentialPath, s.AdmissionCredentialPath} {
		if len(p) > 4096 || !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n\t") {
			return nil, ErrUnavailable
		}
	}
	args := []string{"--candidate-remote-pool", "--data-dir", s.ExistingRoot,
		"--remote-attachment-root", s.AttachmentRoot, "--remote-config", s.ConfigPath,
		"--remote-node-rpc", s.NodeRPCPath, "--remote-pool-credential", s.PoolCredentialPath,
		"--remote-admission-credential", s.AdmissionCredentialPath}
	defaults, e := pilot.ResolvePoolOptions(pilot.PoolOptions{})
	if e != nil {
		return nil, ErrUnavailable
	}
	if options.CandidateDescriptorID != defaults.CandidateDescriptorID {
		args = append(args, "--candidate-name-version=2")
	}
	return exec.Command(helper, args...), nil
}

// LaunchRemotePool owns one new process. Admission context cancellation never
// becomes the lifetime of a successfully returned helper or signed-in session.
func LaunchRemotePool(ctx context.Context, helper string, selection RemotePoolLaunchSelection, expected pilot.RemotePoolConfig) (*RemotePoolBridge, error) {
	if ctx == nil {
		return nil, ErrUnavailable
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	options, e := pilot.ResolvePoolOptions(selection.Options)
	if e != nil || options.CandidateDescriptorID != expected.ExpectedLocal.Recipient.Config.Source.AuthorityPin.DescriptorID {
		return nil, ErrUnavailable
	}
	cmd, e := remotePoolLaunchCommand(helper, selection)
	if e != nil {
		return nil, e
	}
	return startRemotePoolCommand(ctx, cmd, expected)
}

// Unexported: tests may launch their own bounded protocol fixture process. No
// caller can adopt arbitrary inherited pipes or a merely matching process ID.
func startRemotePoolCommand(ctx context.Context, cmd *exec.Cmd, expected pilot.RemotePoolConfig) (out *RemotePoolBridge, resultErr error) {
	if ctx == nil || cmd == nil {
		return nil, ErrUnavailable
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	raw, e := pilot.MarshalRemotePoolConfig(expected)
	if e != nil {
		return nil, ErrUnavailable
	}
	owned, e := pilot.ParseRemotePoolConfig(raw)
	if e != nil {
		return nil, ErrUnavailable
	}
	id, e := pilot.RemotePoolConfigID(owned)
	if e != nil {
		return nil, ErrUnavailable
	}
	b := &RemotePoolBridge{cmd: cmd, config: owned, configID: id, gate: make(chan struct{}, 1),
		lines: make(chan []byte, 1), failed: make(chan struct{}), readDone: make(chan struct{}), waitDone: make(chan struct{})}
	started, readers := false, false
	defer func() {
		if resultErr == nil {
			return
		}
		if !started {
			close(b.waitDone)
		}
		if !readers {
			close(b.readDone)
		}
		b.retireRemote(resultErr)
		cleanup := b.joinRemoteCleanup()
		if cleanup != nil || started {
			out = b
		}
		resultErr = remotePoolFailure("remote_unavailable", errors.Join(resultErr, cleanup))
	}()
	childInput, input, e := os.Pipe()
	if e != nil {
		return nil, ErrUnavailable
	}
	b.pipes = append(b.pipes, remotePoolPipe{owner: childInput}, remotePoolPipe{owner: input})
	b.input = input
	output, childOutput, e := os.Pipe()
	if e != nil {
		return nil, ErrUnavailable
	}
	b.pipes = append(b.pipes, remotePoolPipe{owner: output}, remotePoolPipe{owner: childOutput})
	b.output = output
	// These are our descriptors, not Cmd's StdinPipe/StdoutPipe: Wait must not
	// retry Close on a descriptor whose earlier failure we retained.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = childInput, childOutput, io.Discard
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if e = cmd.Start(); e != nil {
		return nil, ErrUnavailable
	}
	started = true
	b.closeRemotePipe(0)
	b.closeRemotePipe(3)
	readers = true
	go b.readRemote()
	go func() {
		e := cmd.Wait()
		b.mu.Lock()
		b.waitErr = e
		b.mu.Unlock()
		b.retireRemote(ErrUnavailable)
		close(b.waitDone)
	}()
	b.mu.Lock()
	cleanup := b.cleanupErr
	b.mu.Unlock()
	if cleanup != nil {
		return b, cleanup
	}
	initial, e := b.Snapshot(ctx, nil)
	if e != nil {
		return b, e
	}
	if initial.State != "signed_out" || initial.View != nil || initial.Remote != nil || initial.Error != "" {
		return b, ErrUnavailable
	}
	return b, nil
}

func (b *RemotePoolBridge) readRemote() {
	defer close(b.readDone)
	reader := bufio.NewReaderSize(b.output, pilot.MaxRemotePoolResponseSize+1)
	for {
		line, e := reader.ReadSlice('\n')
		// ReadSlice requires the terminal LF, unlike Scanner's final EOF token.
		if e != nil || len(line) < 2 || len(line) > pilot.MaxRemotePoolResponseSize+1 || bytes.IndexByte(line, '\r') >= 0 {
			b.retireRemote(ErrUnavailable)
			return
		}
		raw := append([]byte(nil), line[:len(line)-1]...)
		b.mu.Lock()
		if b.retired {
			b.mu.Unlock()
			return
		}
		if b.awaiting == 0 || b.delivered {
			b.mu.Unlock()
			b.retireRemote(ErrUnavailable)
			return
		}
		b.delivered = true
		select {
		case b.lines <- raw:
			b.mu.Unlock()
		default:
			b.mu.Unlock()
			b.retireRemote(ErrUnavailable)
			return
		}
	}
}

var _ io.Closer = (*os.File)(nil)
