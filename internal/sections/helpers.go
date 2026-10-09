package sections

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zahidhasann88/whereami/internal/env"
)

// Root discovery prefers .git over manifests in the same directory.
var rootMarkers = []string{
	".git", "go.mod", "package.json", "pyproject.toml", "requirements.txt",
	"Pipfile", "Cargo.toml", "pom.xml", "build.gradle", "build.gradle.kts",
	"composer.json", "Gemfile", "setup.py",
}

// findRoot returns the nearest root and marker kind, or empty strings when absent.
func findRoot(fsys env.FS, dir string) (root, source string) {
	cur := filepath.Clean(dir)
	for {
		if src := markerIn(fsys, cur); src != "" {
			return cur, src
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", ""
		}
		cur = parent
	}
}

func markerIn(fsys env.FS, dir string) string {
	names := listNames(fsys, dir)
	if names[".git"] {
		return "git"
	}
	for _, m := range rootMarkers[1:] {
		if names[m] {
			return "marker"
		}
	}
	for n := range names {
		if strings.HasSuffix(n, ".csproj") || strings.HasSuffix(n, ".sln") {
			return "marker"
		}
	}
	return ""
}

// listNames treats unreadable directories as having no markers.
func listNames(fsys env.FS, dir string) map[string]bool {
	entries, err := fsys.ReadDir(dir)
	names := make(map[string]bool, len(entries))
	if err != nil {
		return names
	}
	for _, e := range entries {
		names[e.Name()] = true
	}
	return names
}

// readText returns the file contents, or false if the file cannot be read.
func readText(fsys env.FS, path string) (string, bool) {
	b, err := fsys.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// resolvePath resolves symlinks, falling back to the cleaned path on failure.
func resolvePath(p string) string {
	clean := filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(clean); err == nil {
		return r
	}
	return clean
}

// within reports whether child is root or lies beneath it.
func within(child, root string) bool {
	c, r := resolvePath(child), resolvePath(root)
	if runtime.GOOS == "windows" {
		c, r = strings.ToLower(c), strings.ToLower(r)
	}
	if c == r {
		return true
	}
	rel, err := filepath.Rel(r, c)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
