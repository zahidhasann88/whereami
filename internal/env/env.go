// Package env defines system dependencies used by sections.
package env

import (
	"context"
	"io/fs"
	"time"
)

// Output holds what a command wrote to its standard streams.
type Output struct {
	Stdout []byte
	Stderr []byte
}

// Runner wraps exec.ExitError for command failures and exec.ErrNotFound for missing binaries.
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) (Output, error)
}

// FS reads files using normal os path semantics.
type FS interface {
	Stat(name string) (fs.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

// Listener is one listening socket together with the process that owns it.
type Listener struct {
	IP      string
	Port    int
	Proto   string
	PID     int32
	Process string
	Cwd     string
	// CwdErr records an unreadable process working directory.
	CwdErr error
}

// ProcessSource lists listening sockets.
type ProcessSource interface {
	Listeners(ctx context.Context) ([]Listener, error)
}

// DockerAPI returns Engine GET responses; errors mean Docker is unavailable.
type DockerAPI interface {
	Get(ctx context.Context, path string) ([]byte, error)
}

// Env bundles every dependency a section may use, plus per-run options.
type Env struct {
	// Dir is the absolute directory being inspected.
	Dir       string
	Runner    Runner
	FS        FS
	Processes ProcessSource
	// Docker is nil when Docker is disabled with --no-docker.
	Docker DockerAPI
	// ShowEnvKeys enables key names only, never values.
	ShowEnvKeys bool
	// Now returns the current time; injectable for deterministic output.
	Now func() time.Time
}
