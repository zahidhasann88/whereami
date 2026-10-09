// Package envtest provides fake implementations of the env interfaces for tests.
package envtest

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/zahidhasann88/whereami/internal/env"
)

// Reply is a recorded response for one command invocation.
type Reply struct {
	Stdout string
	Stderr string
	Err    error
}

// Runner matches recorded commands; missing replies return exec.ErrNotFound.
type Runner struct {
	mu      sync.Mutex
	Replies map[string]Reply
	Calls   []string
}

// NewRunner returns a Runner seeded with the given replies.
func NewRunner(replies map[string]Reply) *Runner {
	return &Runner{Replies: replies}
}

func (r *Runner) Run(_ context.Context, _, name string, args ...string) (env.Output, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.mu.Lock()
	r.Calls = append(r.Calls, key)
	rep, ok := r.Replies[key]
	r.mu.Unlock()
	if !ok {
		return env.Output{}, &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
	out := env.Output{Stdout: []byte(rep.Stdout), Stderr: []byte(rep.Stderr)}
	if rep.Err != nil {
		return out, fmt.Errorf("%s: %w", name, rep.Err)
	}
	return out, nil
}

// Processes is a fake env.ProcessSource.
type Processes struct {
	Items []env.Listener
	Err   error
}

func (p Processes) Listeners(context.Context) ([]env.Listener, error) {
	return p.Items, p.Err
}

// Docker is a fake env.DockerAPI keyed by request path.
type Docker struct {
	Responses map[string][]byte
	Err       error
}

func (d Docker) Get(_ context.Context, path string) ([]byte, error) {
	if d.Err != nil {
		return nil, d.Err
	}
	b, ok := d.Responses[path]
	if !ok {
		return nil, fmt.Errorf("docker fake: no response for %s", path)
	}
	return b, nil
}
