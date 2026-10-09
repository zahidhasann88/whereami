package sections

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zahidhasann88/whereami/internal/env"
)

// Runtime compares installed versions with project pins.
type Runtime struct{}

func (Runtime) Name() string { return "runtime" }

// Pin statuses.
const (
	PinMatch    = "match"
	PinMismatch = "mismatch"
	PinMissing  = "missing"
	PinUnknown  = "unknown"
)

// RuntimeData is the data of the runtime section.
type RuntimeData struct {
	Runtimes []RuntimeInfo `json:"runtimes"`
	Warnings []string      `json:"warnings"`
}

// RuntimeInfo describes one runtime.
type RuntimeInfo struct {
	Name    string    `json:"name"`
	Version string    `json:"version,omitempty"`
	Reason  string    `json:"reason,omitempty"`
	Pins    []PinInfo `json:"pins,omitempty"`
}

// PinInfo is a version requirement declared by a project file.
type PinInfo struct {
	Source string `json:"source"`
	Expr   string `json:"expr"`
	Status string `json:"status"`
}

// runtimeOrder fixes display order; pins may only refer to these names.
var runtimeOrder = []string{"go", "node", "npm", "pnpm", "yarn", "bun", "python", "rustc", "java", "php", "ruby", "dotnet"}

type pin struct {
	runtime string
	source  string
	expr    string
}

// probe tries version commands in order until one runs.
type probe struct {
	candidates [][]string
	// re extracts one version group; nil uses the first dotted number.
	re *regexp.Regexp
}

var probes = map[string]probe{
	"go":     {candidates: [][]string{{"go", "version"}}},
	"node":   {candidates: [][]string{{"node", "--version"}}},
	"npm":    {candidates: [][]string{{"npm", "--version"}}},
	"pnpm":   {candidates: [][]string{{"pnpm", "--version"}}},
	"yarn":   {candidates: [][]string{{"yarn", "--version"}}},
	"bun":    {candidates: [][]string{{"bun", "--version"}}},
	"python": {candidates: [][]string{{"python3", "--version"}, {"python", "--version"}}, re: regexp.MustCompile(`Python\s+(\S+)`)},
	"rustc":  {candidates: [][]string{{"rustc", "--version"}}, re: regexp.MustCompile(`rustc\s+(\S+)`)},
	"java":   {candidates: [][]string{{"java", "-version"}}, re: regexp.MustCompile(`version "([^"]+)"`)},
	"php":    {candidates: [][]string{{"php", "-v"}}, re: regexp.MustCompile(`PHP\s+(\S+)`)},
	"ruby":   {candidates: [][]string{{"ruby", "-v"}}, re: regexp.MustCompile(`ruby\s+(\S+)`)},
	"dotnet": {candidates: [][]string{{"dotnet", "--version"}}, re: regexp.MustCompile(`^(\S+)`)},
}

// probeTimeout prevents slow version commands from hiding other runtimes.
const probeTimeout = 1500 * time.Millisecond

func (Runtime) Collect(ctx context.Context, e *env.Env) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	root, _ := findRoot(e.FS, e.Dir)
	if root == "" {
		root = e.Dir
	}
	det := detectProject(e.FS, root)
	pins := collectPins(e.FS, root, det)
	needed := neededRuntimes(det, pins)

	infos := make([]RuntimeInfo, len(needed))
	var wg sync.WaitGroup
	for i, name := range needed {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			infos[i] = probeRuntime(ctx, e, name)
		}(i, name)
	}
	wg.Wait()

	var warnings []string
	for i := range infos {
		name := infos[i].Name
		for _, p := range pins {
			if p.runtime != name {
				continue
			}
			ps := evaluatePin(infos[i], p)
			infos[i].Pins = append(infos[i].Pins, ps)
			switch ps.Status {
			case PinMismatch:
				warnings = append(warnings, fmt.Sprintf("%s %s does not satisfy %s (%s)", name, infos[i].Version, ps.Expr, ps.Source))
			case PinMissing:
				warnings = append(warnings, fmt.Sprintf("%s is pinned to %s by %s but is not installed", name, ps.Expr, ps.Source))
			}
		}
	}
	return Result{Status: StatusOK, Data: RuntimeData{Runtimes: infos, Warnings: nonNilStrings(warnings)}}, nil
}

// neededRuntimes returns the runtimes to probe, in display order.
func neededRuntimes(det projectDetection, pins []pin) []string {
	want := map[string]bool{}
	for _, t := range det.types {
		switch t {
		case "Go":
			want["go"] = true
		case "Node.js", "TypeScript":
			want["node"], want["npm"] = true, true
		case "Python":
			want["python"] = true
		case "Rust":
			want["rustc"] = true
		case "Java":
			want["java"] = true
		case "PHP":
			want["php"] = true
		case "Ruby":
			want["ruby"] = true
		case ".NET":
			want["dotnet"] = true
		}
	}
	for _, pm := range det.packageManagers {
		switch pm {
		case "pnpm", "yarn", "bun":
			want[pm] = true
		}
	}
	for _, p := range pins {
		want[p.runtime] = true
	}
	var out []string
	for _, n := range runtimeOrder {
		if want[n] {
			out = append(out, n)
		}
	}
	return out
}

// probeRuntime runs the version command for one runtime.
func probeRuntime(ctx context.Context, e *env.Env, name string) RuntimeInfo {
	info := RuntimeInfo{Name: name}
	pr, ok := probes[name]
	if !ok {
		info.Reason = "no version probe"
		return info
	}
	allMissing := true
	var lastErr error
	for _, cmd := range pr.candidates {
		cctx, cancel := context.WithTimeout(ctx, probeTimeout)
		out, err := e.Runner.Run(cctx, e.Dir, cmd[0], cmd[1:]...)
		cancel()
		if err != nil {
			if !errors.Is(err, exec.ErrNotFound) {
				allMissing = false
			}
			lastErr = err
			continue
		}
		text := string(out.Stdout) + "\n" + string(out.Stderr)
		if v := extractVersion(pr.re, text); v != "" {
			info.Version = v
			return info
		}
		allMissing = false
		lastErr = fmt.Errorf("unrecognised output from %s", cmd[0])
	}
	switch {
	case allMissing:
		info.Reason = "not installed"
	case errors.Is(lastErr, context.DeadlineExceeded):
		info.Reason = "version command timed out"
	default:
		info.Reason = firstLine(lastErr)
	}
	return info
}

func extractVersion(re *regexp.Regexp, text string) string {
	if re == nil {
		return versionRe.FindString(text)
	}
	m := re.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return versionRe.FindString(m[1])
}

// evaluatePin compares an installed runtime with one pin.
func evaluatePin(info RuntimeInfo, p pin) PinInfo {
	out := PinInfo{Source: p.source, Expr: p.expr}
	if info.Version == "" {
		if info.Reason == "not installed" {
			out.Status = PinMissing
		} else {
			out.Status = PinUnknown
		}
		return out
	}
	v, ok := parseVersion(info.Version)
	if !ok {
		out.Status = PinUnknown
		return out
	}
	match, known := satisfies(v, p.expr)
	switch {
	case !known:
		out.Status = PinUnknown
	case match:
		out.Status = PinMatch
	default:
		out.Status = PinMismatch
	}
	return out
}

// collectPins reads version requirements from project files.
func collectPins(fsys env.FS, root string, det projectDetection) []pin {
	var pins []pin
	names := listNames(fsys, root)
	read := func(f string) string {
		s, _ := readText(fsys, filepath.Join(root, f))
		return s
	}
	firstLineOf := func(s string) string {
		for _, l := range strings.Split(s, "\n") {
			if t := strings.TrimSpace(l); t != "" {
				return t
			}
		}
		return ""
	}
	has := func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}

	if has(det.manifests, "go.mod") {
		if m := regexp.MustCompile(`(?m)^go\s+(\S+)`).FindStringSubmatch(read("go.mod")); m != nil {
			pins = append(pins, pin{"go", "go.mod", ">=" + m[1]})
		}
	}
	if names["Cargo.toml"] {
		if m := regexp.MustCompile(`(?m)^rust-version\s*=\s*"([^"]+)"`).FindStringSubmatch(read("Cargo.toml")); m != nil {
			pins = append(pins, pin{"rustc", "Cargo.toml (rust-version)", ">=" + m[1]})
		}
	}
	for _, f := range []string{".nvmrc", ".node-version"} {
		if names[f] {
			if v := firstLineOf(read(f)); v != "" {
				pins = append(pins, pin{"node", f, strings.TrimPrefix(v, "v")})
			}
		}
	}
	if has(det.manifests, "package.json") {
		var pkg struct {
			Engines        map[string]string `json:"engines"`
			PackageManager string            `json:"packageManager"`
		}
		if raw := read("package.json"); raw != "" && json.Unmarshal([]byte(raw), &pkg) == nil {
			if r, ok := pkg.Engines["node"]; ok {
				pins = append(pins, pin{"node", "package.json (engines.node)", r})
			}
			if name, ver, ok := strings.Cut(pkg.PackageManager, "@"); ok {
				ver, _, _ = strings.Cut(ver, "+")
				if _, known := probes[name]; known && (name == "pnpm" || name == "yarn" || name == "npm" || name == "bun") {
					pins = append(pins, pin{name, "package.json (packageManager)", ver})
				}
			}
		}
	}
	if names[".python-version"] {
		if v := firstLineOf(read(".python-version")); v != "" {
			pins = append(pins, pin{"python", ".python-version", v})
		}
	}
	if names["pyproject.toml"] {
		if m := regexp.MustCompile(`(?m)^requires-python\s*=\s*"([^"]+)"`).FindStringSubmatch(read("pyproject.toml")); m != nil {
			pins = append(pins, pin{"python", "pyproject.toml (requires-python)", m[1]})
		}
	}
	if names["rust-toolchain"] || names["rust-toolchain.toml"] {
		if names["rust-toolchain"] {
			if v := firstLineOf(read("rust-toolchain")); v != "" {
				pins = append(pins, pin{"rustc", "rust-toolchain", v})
			}
		}
		if m := regexp.MustCompile(`(?m)^channel\s*=\s*"([^"]+)"`).FindStringSubmatch(read("rust-toolchain.toml")); m != nil {
			pins = append(pins, pin{"rustc", "rust-toolchain.toml", m[1]})
		}
	}
	if has(det.manifests, "composer.json") {
		var comp struct {
			Require map[string]string `json:"require"`
		}
		if raw := read("composer.json"); raw != "" && json.Unmarshal([]byte(raw), &comp) == nil {
			if r, ok := comp.Require["php"]; ok {
				pins = append(pins, pin{"php", "composer.json (require.php)", r})
			}
		}
	}
	if names[".ruby-version"] {
		if v := firstLineOf(read(".ruby-version")); v != "" {
			pins = append(pins, pin{"ruby", ".ruby-version", v})
		}
	}
	if names[".java-version"] {
		if v := firstLineOf(read(".java-version")); v != "" {
			pins = append(pins, pin{"java", ".java-version", v})
		}
	}
	if names["global.json"] {
		var gj struct {
			SDK struct {
				Version string `json:"version"`
			} `json:"sdk"`
		}
		if raw := read("global.json"); raw != "" && json.Unmarshal([]byte(raw), &gj) == nil && gj.SDK.Version != "" {
			// SDK bands roll forward within a minor line, so pin major.minor.
			parts := strings.Split(gj.SDK.Version, ".")
			if len(parts) >= 2 {
				pins = append(pins, pin{"dotnet", "global.json (sdk.version)", parts[0] + "." + parts[1]})
			}
		}
	}
	return pins
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
