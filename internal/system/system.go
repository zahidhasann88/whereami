// Package system implements the env interfaces.
package system

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"time"

	"github.com/zahidhasann88/whereami/internal/env"
)

// OSRunner runs commands with stable locale and noninteractive, read-only Git settings.
type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, dir, name string, args ...string) (env.Output, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // name is chosen by whereami, args are fixed literals
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"LC_ALL=C",
		"LANG=C",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
	)
	// Do not let a grandchild that inherited the pipes hang Wait forever.
	cmd.WaitDelay = 500 * time.Millisecond
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := env.Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err != nil {
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// OSFS reads the real filesystem.
type OSFS struct{}

func (OSFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }

func (OSFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

func (OSFS) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }

// IsTerminal reports whether f is a character device (an interactive terminal).
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
