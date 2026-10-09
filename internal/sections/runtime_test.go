package sections

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zahidhasann88/whereami/internal/env"
	"github.com/zahidhasann88/whereami/internal/env/envtest"
	"github.com/zahidhasann88/whereami/internal/system"
)

func TestParseVersion(t *testing.T) {
	cases := map[string][]int{
		"go1.23.4 linux/amd64":                {1, 23, 4},
		"v20.11.0":                            {20, 11, 0},
		"rustc 1.75.0 (82e1608df 2023-12-21)": {1, 75, 0},
		"PHP 8.3.4 (cli)":                     {8, 3, 4},
		"3.12":                                {3, 12},
	}
	for in, want := range cases {
		got, ok := parseVersion(in)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("parseVersion(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := parseVersion("unknown"); ok {
		t.Error("expected no version in 'unknown'")
	}
}

func TestSatisfies(t *testing.T) {
	cases := []struct {
		version string
		expr    string
		ok      bool
		known   bool
	}{
		// prefix / bare
		{"18.17.0", "18", true, true},
		{"20.11.0", "18", false, true},
		{"18.17.0", "18.17", true, true},
		{"18.17.1", "18.17", true, true},
		{"18.17.1", "18.17.0", false, true},
		{"3.12.1", "3.12", true, true},
		{"3.11.9", "3.12", false, true},
		{"18.17.0", "18.x", true, true},
		{"18.17.0", "*", true, true},
		// comparators
		{"1.23.4", ">=1.22", true, true},
		{"1.21.9", ">=1.22", false, true},
		{"1.22.0", ">1.22", false, true},
		{"1.23.0", ">1.22", true, true},
		{"3.9.6", ">=3.10", false, true},
		{"3.12.1", ">=3.10,<4", true, true},
		{"4.0.0", ">=3.10,<4", false, true},
		{"20.11.0", ">=18 <21", true, true},
		{"21.0.0", ">=18 <21", false, true},
		{"19.0.0", ">=18 || <16", true, true},
		{"17.0.0", ">=18 || <16", false, true},
		{"1.2.3", "!=1.2.3", false, true},
		{"8.3.4", "^8.2", true, true},
		{"9.0.0", "^8.2", false, true},
		{"0.2.9", "^0.2.3", true, true},
		{"0.3.0", "^0.2.3", false, true},
		{"1.2.9", "~1.2.3", true, true},
		{"1.3.0", "~1.2.3", false, true},
		{"3.10.5", "~=3.10", true, true},
		{"3.11.0", "~=3.10", true, true},
		{"4.0.0", "~=3.10", false, true},
		// unknown syntax is reported as unverifiable, not as a mismatch
		{"20.11.0", "lts/*", false, false},
		{"20.11.0", "stable", false, false},
		{"20.11.0", "", true, true},
	}
	for _, tc := range cases {
		v, ok := parseVersion(tc.version)
		if !ok {
			t.Fatalf("bad test version %q", tc.version)
		}
		got, known := satisfies(v, tc.expr)
		if known != tc.known || (known && got != tc.ok) {
			t.Errorf("satisfies(%s, %q) = (%v, %v); want (%v, %v)", tc.version, tc.expr, got, known, tc.ok, tc.known)
		}
	}
}

// writeFiles creates files under a fresh temp directory.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func findPins(pins []pin, runtime string) []pin {
	var out []pin
	for _, p := range pins {
		if p.runtime == runtime {
			out = append(out, p)
		}
	}
	return out
}

func TestCollectPinsNodeAndGo(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		".nvmrc": "v18.17.0\n",
		"package.json": `{
  "name": "web",
  "engines": {"node": ">=18 <21"},
  "packageManager": "pnpm@8.6.0+sha256.abc"
}`,
		"go.mod": "module example.com/x\n\ngo 1.22\n",
	})
	det := detectProject(system.OSFS{}, dir)
	pins := collectPins(system.OSFS{}, dir, det)

	node := findPins(pins, "node")
	if len(node) != 2 {
		t.Fatalf("node pins = %+v", node)
	}
	if node[0].source != ".nvmrc" || node[0].expr != "18.17.0" {
		t.Errorf("nvmrc pin = %+v", node[0])
	}
	if node[1].source != "package.json (engines.node)" || node[1].expr != ">=18 <21" {
		t.Errorf("engines pin = %+v", node[1])
	}
	pnpm := findPins(pins, "pnpm")
	if len(pnpm) != 1 || pnpm[0].expr != "8.6.0" {
		t.Errorf("pnpm pin = %+v", pnpm)
	}
	goPins := findPins(pins, "go")
	if len(goPins) != 1 || goPins[0].expr != ">=1.22" {
		t.Errorf("go pin = %+v", goPins)
	}
}

func TestCollectPinsOtherRuntimes(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		".python-version":     "3.12\n",
		"pyproject.toml":      "[project]\nname = \"x\"\nrequires-python = \">=3.10\"\n",
		"rust-toolchain.toml": "[toolchain]\nchannel = \"1.75\"\n",
		"Cargo.toml":          "[package]\nname = \"y\"\nrust-version = \"1.70\"\n",
		"composer.json":       `{"require": {"php": "^8.2"}}`,
		".ruby-version":       "3.3.0\n",
		".java-version":       "21\n",
		"global.json":         `{"sdk": {"version": "8.0.100"}}`,
	})
	det := detectProject(system.OSFS{}, dir)
	pins := collectPins(system.OSFS{}, dir, det)
	want := map[string]string{
		"python": "3.12",
		"rustc":  "1.75",
		"php":    "^8.2",
		"ruby":   "3.3.0",
		"java":   "21",
		"dotnet": "8.0",
	}
	for rt, expr := range want {
		found := false
		for _, p := range findPins(pins, rt) {
			if p.expr == expr {
				found = true
			}
		}
		if !found {
			t.Errorf("missing pin %s %s in %+v", rt, expr, pins)
		}
	}
	if len(findPins(pins, "python")) != 2 {
		t.Errorf("expected python pins from .python-version and pyproject, got %+v", findPins(pins, "python"))
	}
}

func TestRuntimeCollectMismatchAndMissing(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		".nvmrc":         "18\n",
		"go.mod":         "module example.com/x\n\ngo 1.22\n",
		"pyproject.toml": "[project]\nname = \"x\"\nrequires-python = \">=3.10\"\n",
	})
	runner := envtest.NewRunner(map[string]envtest.Reply{
		"node --version": {Stdout: "v20.11.0\n"},
		"go version":     {Stdout: "go version go1.23.4 linux/amd64\n"},
	})
	e := &env.Env{Dir: dir, Runner: runner, FS: system.OSFS{}}
	res, err := Runtime{}.Collect(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	data := res.Data.(RuntimeData)
	byName := map[string]RuntimeInfo{}
	for _, r := range data.Runtimes {
		byName[r.Name] = r
	}

	node := byName["node"]
	if node.Version != "20.11.0" || len(node.Pins) != 1 || node.Pins[0].Status != PinMismatch {
		t.Errorf("node = %+v", node)
	}
	if g := byName["go"]; g.Version != "1.23.4" || len(g.Pins) != 1 || g.Pins[0].Status != PinMatch {
		t.Errorf("go = %+v", g)
	}
	py := byName["python"]
	if py.Version != "" || py.Reason != "not installed" || len(py.Pins) != 1 || py.Pins[0].Status != PinMissing {
		t.Errorf("python = %+v", py)
	}
	if len(data.Warnings) != 2 {
		t.Errorf("warnings = %v", data.Warnings)
	}
	if _, ok := byName["npm"]; ok {
		t.Error("npm should not be probed without a Node project")
	}
}

func TestRuntimeCollectNoProject(t *testing.T) {
	dir := t.TempDir()
	e := &env.Env{Dir: dir, Runner: envtest.NewRunner(nil), FS: system.OSFS{}}
	res, _ := Runtime{}.Collect(context.Background(), e)
	data := res.Data.(RuntimeData)
	if len(data.Runtimes) != 0 || res.Status != StatusOK {
		t.Errorf("expected no runtimes, got %+v", data)
	}
}

func TestRuntimeCollectNodeProjectProbesNpm(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"package.json":   `{"name":"x","packageManager":"pnpm@8.6.0"}`,
		"pnpm-lock.yaml": "lockfileVersion: 9\n",
	})
	runner := envtest.NewRunner(map[string]envtest.Reply{
		"node --version": {Stdout: "v20.11.0\n"},
		"npm --version":  {Stdout: "10.2.4\n"},
		"pnpm --version": {Stdout: "8.6.0\n"},
	})
	res, _ := Runtime{}.Collect(context.Background(), &env.Env{Dir: dir, Runner: runner, FS: system.OSFS{}})
	data := res.Data.(RuntimeData)
	var names []string
	for _, r := range data.Runtimes {
		names = append(names, r.Name)
	}
	if !reflect.DeepEqual(names, []string{"node", "npm", "pnpm"}) {
		t.Errorf("names = %v", names)
	}
	if len(data.Warnings) != 0 {
		t.Errorf("unexpected warnings %v", data.Warnings)
	}
}
