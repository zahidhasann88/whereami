// Package cli wires commands, sections and renderers.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zahidhasann88/whereami/internal/env"
	"github.com/zahidhasann88/whereami/internal/render"
	"github.com/zahidhasann88/whereami/internal/sections"
	"github.com/zahidhasann88/whereami/internal/system"
)

// Exit codes.
const (
	ExitOK      = 0
	ExitUsage   = 2
	ExitRuntime = 3
)

// DefaultTimeout is the per-section time budget.
const DefaultTimeout = 2 * time.Second

// BuildInfo is injected at build time via -ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Deps are the injectable dependencies of a run.
type Deps struct {
	Runner     env.Runner
	FS         env.FS
	Processes  env.ProcessSource
	Docker     env.DockerAPI // nil disables Docker lookups
	Registry   *sections.Registry
	Now        func() time.Time
	LookupEnv  func(key string) (string, bool)
	IsTerminal bool // whether stdout is an interactive terminal
}

// SystemDeps returns the production dependencies.
func SystemDeps(stdout io.Writer) Deps {
	isTTY := false
	if f, ok := stdout.(*os.File); ok {
		isTTY = system.IsTerminal(f)
	}
	return Deps{
		Runner:     system.OSRunner{},
		FS:         system.OSFS{},
		Processes:  system.GopsutilProcesses{},
		Docker:     system.NewDockerClient(),
		Registry:   sections.Builtin(),
		Now:        time.Now,
		LookupEnv:  os.LookupEnv,
		IsTerminal: isTTY,
	}
}

// Execute runs the CLI with production dependencies and returns the exit code.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, info BuildInfo) int {
	return Run(ctx, args, stdout, stderr, info, SystemDeps(stdout))
}

// Run runs the CLI with the given dependencies and returns the exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, info BuildInfo, deps Deps) int {
	if deps.Registry == nil {
		deps.Registry = sections.Builtin()
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.LookupEnv == nil {
		deps.LookupEnv = func(string) (string, bool) { return "", false }
	}
	root := newRootCommand(info, deps, stdout, stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	var ue *usageError
	if errors.As(err, &ue) {
		_, _ = fmt.Fprintf(stderr, "whereami: %v\nTry 'whereami --help' for usage.\n", ue.err)
		return ExitUsage
	}
	_, _ = fmt.Fprintf(stderr, "whereami: %v\n", err)
	return ExitRuntime
}

type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

func usagef(format string, a ...any) error {
	return &usageError{err: fmt.Errorf(format, a...)}
}

type options struct {
	asJSON      bool
	noColor     bool
	noDocker    bool
	showEnvKeys bool
	path        string
	timeout     time.Duration
}

func newRootCommand(info BuildInfo, deps Deps, stdout, stderr io.Writer) *cobra.Command {
	var opts options
	names := deps.Registry.Names()

	cmd := &cobra.Command{
		Use:   "whereami [section]",
		Short: "Print a concise summary of the project you are standing in",
		Long: "whereami prints the project type, Git state, runtime versions, listening\n" +
			"ports, containers and configuration files for a directory.\n\n" +
			"Sections: " + strings.Join(names, ", ") + ".",
		Example: "  whereami\n  whereami git\n  whereami --json --path ~/code/api",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usagef("accepts at most one section name, got %d", len(args))
			}
			if len(args) == 1 {
				if _, ok := deps.Registry.Lookup(args[0]); !ok {
					return usagef("unknown section %q (valid: %s)", args[0], strings.Join(names, ", "))
				}
			}
			return nil
		},
		ValidArgs:         names,
		SilenceUsage:      true,
		SilenceErrors:     true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSummary(cmd.Context(), info.Version, opts, args, deps, stdout)
		},
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usagef("%v", err) })

	f := cmd.Flags()
	f.BoolVar(&opts.asJSON, "json", false, "print the stable JSON schema instead of text")
	f.BoolVar(&opts.noColor, "no-color", false, "disable colour output")
	f.BoolVar(&opts.noDocker, "no-docker", false, "do not contact the Docker daemon")
	f.BoolVar(&opts.showEnvKeys, "show-env-keys", false, "list environment variable NAMES (never values)")
	f.StringVar(&opts.path, "path", "", "inspect this directory instead of the current one")
	f.DurationVar(&opts.timeout, "timeout", DefaultTimeout, "per-section time budget")

	// cobra adds --version automatically when Version is set.
	cmd.Version = fmt.Sprintf("%s (commit %s, built %s)", info.Version, info.Commit, info.Date)
	cmd.SetVersionTemplate("whereami {{.Version}}\n")

	_ = cmd.RegisterFlagCompletionFunc("path", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	})
	return cmd
}

func runSummary(ctx context.Context, version string, opts options, args []string, deps Deps, stdout io.Writer) error {
	if opts.timeout <= 0 {
		return usagef("--timeout must be greater than zero")
	}
	dir, err := resolveDir(opts.path, deps.FS)
	if err != nil {
		return err
	}

	secs := deps.Registry.All()
	if len(args) == 1 {
		s, _ := deps.Registry.Lookup(args[0])
		secs = []sections.Section{s}
	}

	docker := deps.Docker
	if opts.noDocker {
		docker = nil
	}
	e := &env.Env{
		Dir:         dir,
		Runner:      deps.Runner,
		FS:          deps.FS,
		Processes:   deps.Processes,
		Docker:      docker,
		ShowEnvKeys: opts.showEnvKeys,
		Now:         deps.Now,
	}
	results := sections.Run(ctx, e, secs, opts.timeout)

	rep := render.Report{
		Version:     version,
		Path:        dir,
		GeneratedAt: deps.Now(),
		Results:     results,
	}
	if opts.asJSON {
		if err := render.JSON(stdout, rep); err != nil {
			return fmt.Errorf("writing JSON: %w", err)
		}
		return nil
	}
	style := textStyle(deps, opts.noColor)
	if err := render.Text(stdout, rep, style); err != nil {
		return fmt.Errorf("writing text: %w", err)
	}
	return nil
}

// resolveDir returns the absolute directory to inspect, validating that it exists.
func resolveDir(path string, fsys env.FS) (string, error) {
	if path == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("determining working directory: %w", err)
		}
		path = wd
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", usagef("invalid --path %q: %v", path, err)
	}
	fi, err := fsys.Stat(abs)
	if err != nil {
		return "", usagef("cannot inspect %q: %v", path, err)
	}
	if !fi.IsDir() {
		return "", usagef("%q is not a directory", path)
	}
	return abs, nil
}

// textStyle applies the colour and emoji rules: both require an interactive
// terminal; NO_COLOR, TERM=dumb and --no-color disable colour.
func textStyle(deps Deps, noColorFlag bool) render.Style {
	if !deps.IsTerminal {
		return render.Style{}
	}
	if term, ok := deps.LookupEnv("TERM"); ok && term == "dumb" {
		return render.Style{}
	}
	style := render.Style{Emoji: true}
	if noColorFlag {
		return style
	}
	if v, ok := deps.LookupEnv("NO_COLOR"); ok && v != "" {
		return style
	}
	style.Color = true
	return style
}
