package sections

import (
	"context"
	"fmt"
	"sort"

	"github.com/zahidhasann88/whereami/internal/env"
)

// Ports lists TCP listeners whose working directories belong to the project.
type Ports struct{}

func (Ports) Name() string { return "ports" }

// PortsInfo is the data of the ports section.
type PortsInfo struct {
	Listening []PortInfo `json:"listening"`
	// Inaccessible counts listeners with unreadable working directories.
	Inaccessible int `json:"inaccessible"`
}

// PortInfo is one listening port owned by a process inside the project.
type PortInfo struct {
	Port      int      `json:"port"`
	Proto     string   `json:"proto"`
	Addresses []string `json:"addresses"`
	PID       int32    `json:"pid"`
	Process   string   `json:"process"`
	Cwd       string   `json:"cwd"`
}

func (Ports) Collect(ctx context.Context, e *env.Env) (Result, error) {
	if e.Processes == nil {
		return unavailable("process listing is not available"), nil
	}
	root, _ := findRoot(e.FS, e.Dir)
	if root == "" {
		root = e.Dir
	}
	listeners, err := e.Processes.Listeners(ctx)
	if err != nil {
		return unavailable(fmt.Sprintf("could not list sockets: %v", err)), nil
	}
	info := PortsInfo{}
	mine := FilterListeners(listeners, root, &info.Inaccessible)
	info.Listening = GroupPorts(mine)

	if len(listeners) > 0 && info.Inaccessible == len(listeners) {
		return unavailable("working directories of listening processes cannot be read (try elevated privileges)"), nil
	}
	return Result{Status: StatusOK, Data: info}, nil
}

// FilterListeners attributes listeners by working directory and counts unreadable owners.
func FilterListeners(listeners []env.Listener, root string, inaccessible *int) []env.Listener {
	var out []env.Listener
	for _, l := range listeners {
		if l.CwdErr != nil || l.Cwd == "" {
			if inaccessible != nil {
				*inaccessible++
			}
			continue
		}
		if within(l.Cwd, root) {
			out = append(out, l)
		}
	}
	return out
}

// GroupPorts merges by port and PID, then sorts by those fields.
func GroupPorts(listeners []env.Listener) []PortInfo {
	type key struct {
		port int
		pid  int32
	}
	idx := map[key]int{}
	var out []PortInfo
	for _, l := range listeners {
		k := key{l.Port, l.PID}
		i, ok := idx[k]
		if !ok {
			idx[k] = len(out)
			out = append(out, PortInfo{
				Port:    l.Port,
				Proto:   l.Proto,
				PID:     l.PID,
				Process: l.Process,
				Cwd:     l.Cwd,
			})
			i = len(out) - 1
		}
		if !containsString(out[i].Addresses, l.IP) {
			out[i].Addresses = append(out[i].Addresses, l.IP)
		}
	}
	for i := range out {
		sort.Strings(out[i].Addresses)
		if out[i].Process == "" {
			out[i].Process = "unknown"
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Port != out[b].Port {
			return out[a].Port < out[b].Port
		}
		return out[a].PID < out[b].PID
	})
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
