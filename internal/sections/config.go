package sections

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zahidhasann88/whereami/internal/env"
)

// Config lists project configuration files without exposing env values.
type Config struct{}

func (Config) Name() string { return "config" }

// Gitignore states for env files.
const (
	GitignoreIgnored    = "ignored"
	GitignoreNotIgnored = "not_ignored"
	GitignoreTracked    = "tracked"
	GitignoreUnknown    = "unknown"
)

// ConfigInfo is the data of the config section.
type ConfigInfo struct {
	EnvFiles []EnvFile    `json:"env_files"`
	Files    []ConfigFile `json:"files"`
	Warnings []string     `json:"warnings"`
}

// EnvFile describes one .env-style file. It never carries values.
type EnvFile struct {
	Name      string   `json:"name"`
	Variables int      `json:"variables"`
	Template  bool     `json:"template,omitempty"`
	Gitignore string   `json:"gitignore,omitempty"`
	Keys      []string `json:"keys,omitempty"`
}

// ConfigFile is a non-env configuration file.
type ConfigFile struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

var (
	envKeyRe       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)
	configNameRe   = regexp.MustCompile(`^config\.(yaml|yml|json|toml)$`)
	dockerfileRe   = regexp.MustCompile(`^(Dockerfile(\..+)?|.+\.Dockerfile)$`)
	makefileNames  = map[string]bool{"Makefile": true, "makefile": true, "GNUmakefile": true}
	ciRootNames    = map[string]bool{".gitlab-ci.yml": true, ".travis.yml": true, "azure-pipelines.yml": true, "bitbucket-pipelines.yml": true, ".drone.yml": true, "Jenkinsfile": true}
	templateSuffix = []string{".example", ".sample", ".template", ".dist"}
)

func (Config) Collect(ctx context.Context, e *env.Env) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	root, _ := findRoot(e.FS, e.Dir)
	if root == "" {
		root = e.Dir
	}
	entries, err := e.FS.ReadDir(root)
	if err != nil {
		return unavailable(fmt.Sprintf("cannot read %s: %v", root, err)), nil
	}

	info := ConfigInfo{EnvFiles: []EnvFile{}, Files: []ConfigFile{}, Warnings: []string{}}
	var envNames []string
	for _, en := range entries {
		if en.IsDir() {
			continue
		}
		name := en.Name()
		switch {
		case name == ".env" || strings.HasPrefix(name, ".env."):
			envNames = append(envNames, name)
		case configNameRe.MatchString(name):
			info.Files = append(info.Files, ConfigFile{Path: name, Kind: "config"})
		case composeFileRe.MatchString(name):
			info.Files = append(info.Files, ConfigFile{Path: name, Kind: "compose"})
		case dockerfileRe.MatchString(name):
			info.Files = append(info.Files, ConfigFile{Path: name, Kind: "dockerfile"})
		case makefileNames[name]:
			info.Files = append(info.Files, ConfigFile{Path: name, Kind: "makefile"})
		case ciRootNames[name]:
			info.Files = append(info.Files, ConfigFile{Path: name, Kind: "ci"})
		}
	}
	sort.Strings(envNames)
	for _, name := range envNames {
		ef := scanEnvFile(e.FS, root, name, e.ShowEnvKeys)
		if !ef.Template {
			ef.Gitignore = gitignoreState(ctx, e, root, name)
			switch ef.Gitignore {
			case GitignoreNotIgnored:
				info.Warnings = append(info.Warnings, fmt.Sprintf("%s is not gitignored; add it to .gitignore before committing", name))
			case GitignoreTracked:
				info.Warnings = append(info.Warnings, fmt.Sprintf("%s is tracked by git; remove it with `git rm --cached %s`", name, name))
			}
		}
		info.EnvFiles = append(info.EnvFiles, ef)
	}

	info.Files = append(info.Files, ciFiles(e.FS, root)...)
	sort.SliceStable(info.Files, func(a, b int) bool {
		if info.Files[a].Kind != info.Files[b].Kind {
			return info.Files[a].Kind < info.Files[b].Kind
		}
		return info.Files[a].Path < info.Files[b].Path
	})
	return Result{Status: StatusOK, Data: info}, nil
}

// scanEnvFile counts assignments and optionally retains keys, never values in its result.
func scanEnvFile(fsys env.FS, root, name string, withKeys bool) EnvFile {
	ef := EnvFile{Name: name, Template: isTemplate(name)}
	content, ok := readText(fsys, filepath.Join(root, name))
	if !ok {
		return ef
	}
	keys := []string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, _, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || !envKeyRe.MatchString(key) {
			continue
		}
		ef.Variables++
		if withKeys {
			keys = append(keys, key)
		}
	}
	if withKeys {
		ef.Keys = keys
	}
	return ef
}

func isTemplate(name string) bool {
	for _, s := range templateSuffix {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}

// gitignoreState returns unknown outside Git repositories or when Git checks fail.
func gitignoreState(ctx context.Context, e *env.Env, root, name string) string {
	tracked, err := e.Runner.Run(ctx, root, "git", "ls-files", "--", name)
	if err != nil {
		return GitignoreUnknown
	}
	if strings.TrimSpace(string(tracked.Stdout)) != "" {
		return GitignoreTracked
	}
	_, err = e.Runner.Run(ctx, root, "git", "check-ignore", "--quiet", "--no-index", "--", name)
	if err == nil {
		return GitignoreIgnored
	}
	// *exec.ExitError (and test doubles) expose the status via ExitCode.
	var ec interface{ ExitCode() int }
	if errors.As(err, &ec) && ec.ExitCode() == 1 {
		return GitignoreNotIgnored
	}
	return GitignoreUnknown
}

// ciFiles finds GitHub Actions and CircleCI definitions below the root.
func ciFiles(fsys env.FS, root string) []ConfigFile {
	var out []ConfigFile
	if entries, err := fsys.ReadDir(filepath.Join(root, ".github", "workflows")); err == nil {
		for _, en := range entries {
			n := en.Name()
			if !en.IsDir() && (strings.HasSuffix(n, ".yml") || strings.HasSuffix(n, ".yaml")) {
				out = append(out, ConfigFile{Path: ".github/workflows/" + n, Kind: "ci"})
			}
		}
	}
	if _, err := fsys.Stat(filepath.Join(root, ".circleci", "config.yml")); err == nil {
		out = append(out, ConfigFile{Path: ".circleci/config.yml", Kind: "ci"})
	}
	return out
}
