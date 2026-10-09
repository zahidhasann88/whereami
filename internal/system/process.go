package system

import (
	"context"
	"fmt"

	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/zahidhasann88/whereami/internal/env"
)

// GopsutilProcesses resolves TCP listeners and their process working directories.
type GopsutilProcesses struct{}

type owner struct {
	name, cwd string
	cwdErr    error
}

func (GopsutilProcesses) Listeners(ctx context.Context) ([]env.Listener, error) {
	conns, err := gnet.ConnectionsWithContext(ctx, "tcp")
	if err != nil {
		return nil, fmt.Errorf("listing sockets: %w", err)
	}
	owners := map[int32]owner{}
	var out []env.Listener
	for _, c := range conns {
		if c.Status != "LISTEN" || c.Pid <= 0 || c.Laddr.Port == 0 {
			continue
		}
		o, seen := owners[c.Pid]
		if !seen {
			o = lookupOwner(ctx, c.Pid)
			owners[c.Pid] = o
		}
		out = append(out, env.Listener{
			IP:      c.Laddr.IP,
			Port:    int(c.Laddr.Port),
			Proto:   "tcp",
			PID:     c.Pid,
			Process: o.name,
			Cwd:     o.cwd,
			CwdErr:  o.cwdErr,
		})
	}
	return out, nil
}

func lookupOwner(ctx context.Context, pid int32) owner {
	var o owner
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		o.cwdErr = err
		return o
	}
	if name, err := p.Name(); err == nil {
		o.name = name
	}
	cwd, err := p.CwdWithContext(ctx)
	if err != nil {
		o.cwdErr = err
		return o
	}
	o.cwd = cwd
	return o
}
