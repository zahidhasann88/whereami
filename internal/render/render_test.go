package render

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zahidhasann88/whereami/internal/sections"
)

var update = flag.Bool("update", false, "rewrite golden files")

// sampleReport is a deterministic report covering every section state.
func sampleReport() Report {
	return Report{
		Version:     "0.1.0",
		Path:        "/home/dev/code/api",
		GeneratedAt: time.Date(2026, 10, 10, 9, 30, 0, 0, time.UTC),
		Results: []sections.Result{
			{Name: "project", Status: sections.StatusOK, Data: sections.ProjectInfo{
				Dir:             "/home/dev/code/api",
				Root:            "/home/dev/code/api",
				RootSource:      "git",
				Name:            "github.com/acme/api",
				Types:           []string{"Go", "Node.js", "TypeScript"},
				Frameworks:      []string{"Gin", "Next.js"},
				PackageManagers: []string{"go modules", "pnpm"},
				Manifests:       []string{"go.mod", "package.json", "tsconfig.json"},
			}},
			{Name: "git", Status: sections.StatusOK, Data: sections.GitInfo{
				Branch:    "feature/login",
				Upstream:  "origin/feature/login",
				Ahead:     2,
				Behind:    1,
				Staged:    1,
				Modified:  3,
				Untracked: 2,
				LastCommit: &sections.CommitInfo{
					Hash: "a1b2c3d", Subject: "Add login handler", RelativeTime: "2 hours ago",
					Date: "2026-10-10T08:00:00+06:00", Author: "Jane Doe",
				},
				Remote:  &sections.RemoteInfo{Name: "origin", URL: "https://github.com/acme/api.git"},
				Stashes: 1,
			}},
			{Name: "runtime", Status: sections.StatusOK, Data: sections.RuntimeData{
				Runtimes: []sections.RuntimeInfo{
					{Name: "go", Version: "1.23.4", Pins: []sections.PinInfo{{Source: "go.mod", Expr: ">=1.22", Status: sections.PinMatch}}},
					{Name: "node", Version: "20.11.0", Pins: []sections.PinInfo{
						{Source: ".nvmrc", Expr: "18", Status: sections.PinMismatch},
					}},
					{Name: "npm", Version: "10.2.4"},
					{Name: "python", Reason: "not installed", Pins: []sections.PinInfo{
						{Source: ".python-version", Expr: "3.12", Status: sections.PinMissing},
					}},
				},
				Warnings: []string{"node 20.11.0 does not satisfy 18 (.nvmrc)"},
			}},
			{Name: "ports", Status: sections.StatusOK, Data: sections.PortsInfo{
				Listening: []sections.PortInfo{
					{Port: 3000, Proto: "tcp", PID: 4812, Process: "node", Addresses: []string{"127.0.0.1", "::1"}},
					{Port: 5432, Proto: "tcp", PID: 2210, Process: "postgres", Addresses: []string{"0.0.0.0"}},
				},
				Inaccessible: 2,
			}},
			{Name: "containers", Status: sections.StatusOK, Data: sections.ContainersInfo{
				DockerAvailable: true,
				ComposeFiles: []sections.ComposeFile{
					{Path: "docker-compose.yml", Project: "api", Services: []string{"web", "db"}},
				},
				Running: []sections.ContainerInfo{
					{ID: "9f1c2a7b3d4e", Name: "api-web-1", Image: "api-web:latest", Status: "Up 3 hours", Ports: []string{"0.0.0.0:8080->8000/tcp"}, MatchedBy: "working_dir"},
					{ID: "aa11bb22cc33", Name: "api-db-1", Image: "postgres:16", Status: "Up 3 hours", Ports: []string{"5432/tcp"}, MatchedBy: "working_dir"},
				},
			}},
			{Name: "config", Status: sections.StatusOK, Data: sections.ConfigInfo{
				EnvFiles: []sections.EnvFile{
					{Name: ".env", Variables: 4, Gitignore: sections.GitignoreNotIgnored, Keys: []string{"DATABASE_URL", "API_KEY", "DEBUG", "PORT"}},
					{Name: ".env.local", Variables: 1, Gitignore: sections.GitignoreIgnored},
					{Name: ".env.example", Variables: 4, Template: true},
				},
				Files: []sections.ConfigFile{
					{Path: "Dockerfile", Kind: "dockerfile"},
					{Path: "docker-compose.yml", Kind: "compose"},
					{Path: "Makefile", Kind: "makefile"},
					{Path: ".github/workflows/ci.yml", Kind: "ci"},
				},
				Warnings: []string{".env is not gitignored; add it to .gitignore before committing"},
			}},
		},
	}
}

// degradedReport exercises unavailable, timed-out and skipped sections.
func degradedReport() Report {
	return Report{
		Version: "0.1.0",
		Path:    "/tmp/scratch",
		Results: []sections.Result{
			{Name: "project", Status: sections.StatusOK, Data: sections.ProjectInfo{
				Dir: "/tmp/scratch", Root: "/tmp/scratch", RootSource: "directory",
				Types: []string{}, Frameworks: []string{}, PackageManagers: []string{}, Manifests: []string{},
			}},
			{Name: "git", Status: sections.StatusUnavailable, Reason: "not a git repository"},
			{Name: "runtime", Status: sections.StatusOK, Data: sections.RuntimeData{Runtimes: []sections.RuntimeInfo{}, Warnings: []string{}}},
			{Name: "ports", Status: sections.StatusTimedOut, Reason: "timed out after 2s"},
			{Name: "containers", Status: sections.StatusSkipped},
			{Name: "config", Status: sections.StatusOK, Data: sections.ConfigInfo{EnvFiles: []sections.EnvFile{}, Files: []sections.ConfigFile{}, Warnings: []string{}}},
		},
	}
}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file (run with -update): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from golden file; run `go test ./internal/render -update` if the change is intended\n--- got ---\n%s", name, got)
	}
}

func TestTextGolden(t *testing.T) {
	var b bytes.Buffer
	if err := Text(&b, sampleReport(), Style{}); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "sample.text.golden", b.Bytes())
}

func TestTextDegradedGolden(t *testing.T) {
	var b bytes.Buffer
	if err := Text(&b, degradedReport(), Style{}); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "degraded.text.golden", b.Bytes())
}

func TestJSONGolden(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, sampleReport()); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "sample.json.golden", b.Bytes())
}

func TestJSONDegradedGolden(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, degradedReport()); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "degraded.json.golden", b.Bytes())
}

func TestPlainTextHasNoEscapesOrEmoji(t *testing.T) {
	var b bytes.Buffer
	if err := Text(&b, sampleReport(), Style{}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "\x1b[") {
		t.Error("plain output contains ANSI escapes")
	}
	for _, sym := range []string{"✓", "⚠", "→"} {
		if strings.Contains(out, sym) {
			t.Errorf("plain output contains %q", sym)
		}
	}
}

func TestColourStyleAddsEscapes(t *testing.T) {
	var b bytes.Buffer
	if err := Text(&b, sampleReport(), Style{Color: true, Emoji: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "\x1b[") {
		t.Error("expected ANSI escapes with colour enabled")
	}
}
