package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zahidhasann88/whereami/internal/env"
	"github.com/zahidhasann88/whereami/internal/env/envtest"
	"github.com/zahidhasann88/whereami/internal/render"
	"github.com/zahidhasann88/whereami/internal/sections"
	"github.com/zahidhasann88/whereami/internal/system"
)

const secret = "s3cr3t-VALUE-must-not-appear-42"

var info = BuildInfo{Version: "1.2.3", Commit: "abc1234", Date: "2026-10-10"}

// testDeps returns fakes for everything except the file system.
func testDeps(processes env.ProcessSource) Deps {
	return Deps{
		Runner:     envtest.NewRunner(nil), // every command is "not installed"
		FS:         system.OSFS{},
		Processes:  processes,
		Docker:     nil,
		Registry:   sections.Builtin(),
		Now:        func() time.Time { return time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC) },
		LookupEnv:  func(string) (string, bool) { return "", false },
		IsTerminal: false,
	}
}

func projectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":   "module example.com/demo\n\ngo 1.22\n",
		"main.go":  "package main\n",
		".env":     "DATABASE_URL=postgres://u:" + secret + "@db/x\nAPI_TOKEN=" + secret + "\n",
		"Makefile": "all:\n\tgo build\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runCLI(t *testing.T, deps Deps, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Run(context.Background(), args, &out, &errOut, info, deps)
	return code, out.String(), errOut.String()
}

func TestExitCodes(t *testing.T) {
	dir := projectDir(t)
	cases := []struct {
		name string
		args []string
		code int
		err  string
	}{
		{"unknown section", []string{"bogus"}, ExitUsage, "unknown section"},
		{"too many args", []string{"git", "ports"}, ExitUsage, "at most one section"},
		{"unknown flag", []string{"--nope"}, ExitUsage, "unknown flag"},
		{"zero timeout", []string{"--timeout=0s", "--path", dir}, ExitUsage, "greater than zero"},
		{"missing path", []string{"--path", filepath.Join(dir, "missing")}, ExitUsage, "cannot inspect"},
		{"path is a file", []string{"--path", filepath.Join(dir, "go.mod")}, ExitUsage, "not a directory"},
		{"full summary", []string{"--path", dir, "--no-docker"}, ExitOK, ""},
		{"help", []string{"--help"}, ExitOK, ""},
		{"version", []string{"--version"}, ExitOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errOut := runCLI(t, testDeps(envtest.Processes{}), tc.args...)
			if code != tc.code {
				t.Errorf("exit = %d, want %d (stderr %q)", code, tc.code, errOut)
			}
			if tc.err != "" && !strings.Contains(errOut, tc.err) {
				t.Errorf("stderr = %q, want it to contain %q", errOut, tc.err)
			}
		})
	}
}

func TestVersionFlag(t *testing.T) {
	_, out, _ := runCLI(t, testDeps(envtest.Processes{}), "--version")
	if out != "whereami 1.2.3 (commit abc1234, built 2026-10-10)\n" {
		t.Errorf("version output = %q", out)
	}
}

func TestRuntimeErrorExitCode(t *testing.T) {
	dir := projectDir(t)
	code := Run(context.Background(), []string{"--path", dir, "--no-docker"}, failingWriter{}, &bytes.Buffer{}, info, testDeps(envtest.Processes{}))
	if code != ExitRuntime {
		t.Errorf("exit = %d, want %d", code, ExitRuntime)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestJSONSchema(t *testing.T) {
	dir := projectDir(t)
	code, out, errOut := runCLI(t, testDeps(envtest.Processes{}), "--json", "--path", dir, "--no-docker")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var doc struct {
		SchemaVersion int    `json:"schema_version"`
		Tool          string `json:"tool"`
		Version       string `json:"version"`
		GeneratedAt   string `json:"generated_at"`
		Path          string `json:"path"`
		Sections      []struct {
			Name   string          `json:"name"`
			Status string          `json:"status"`
			Reason string          `json:"reason"`
			Data   json.RawMessage `json:"data"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if doc.SchemaVersion != render.SchemaVersion || doc.Tool != "whereami" || doc.Version != "1.2.3" {
		t.Errorf("header = %+v", doc)
	}
	if doc.GeneratedAt != "2026-10-10T09:00:00Z" {
		t.Errorf("generated_at = %q", doc.GeneratedAt)
	}
	var names []string
	for _, s := range doc.Sections {
		names = append(names, s.Name)
	}
	want := "project,git,runtime,ports,containers,config"
	if strings.Join(names, ",") != want {
		t.Errorf("sections = %v", names)
	}
	if strings.Contains(out, secret) {
		t.Fatal("secret value present in JSON output")
	}
}

func TestSingleSectionOnly(t *testing.T) {
	dir := projectDir(t)
	_, out, _ := runCLI(t, testDeps(envtest.Processes{}), "--path", dir, "--no-docker", "config")
	if strings.Contains(out, "Project\n") || !strings.Contains(out, "Config\n") {
		t.Errorf("output should contain only Config:\n%s", out)
	}
	if !strings.Contains(out, ".env") || !strings.Contains(out, "2 variables") {
		t.Errorf("config summary missing:\n%s", out)
	}
}

func TestNoEnvValuesAnywhere(t *testing.T) {
	dir := projectDir(t)
	for _, args := range [][]string{
		{"--path", dir, "--no-docker"},
		{"--path", dir, "--no-docker", "--json"},
		{"--path", dir, "--no-docker", "--show-env-keys"},
		{"--path", dir, "--no-docker", "--show-env-keys", "--json"},
	} {
		code, out, errOut := runCLI(t, testDeps(envtest.Processes{}), args...)
		if code != ExitOK {
			t.Fatalf("%v: exit %d %s", args, code, errOut)
		}
		if strings.Contains(out, secret) || strings.Contains(errOut, secret) {
			t.Errorf("%v leaked a secret value", args)
		}
	}
}

func TestShowEnvKeysFlag(t *testing.T) {
	dir := projectDir(t)
	_, without, _ := runCLI(t, testDeps(envtest.Processes{}), "--path", dir, "--no-docker", "config")
	if strings.Contains(without, "API_TOKEN") {
		t.Error("variable names shown without --show-env-keys")
	}
	_, with, _ := runCLI(t, testDeps(envtest.Processes{}), "--path", dir, "--no-docker", "--show-env-keys", "config")
	if !strings.Contains(with, "API_TOKEN") || !strings.Contains(with, "DATABASE_URL") {
		t.Errorf("variable names missing with --show-env-keys:\n%s", with)
	}
}

func TestSectionFailureDoesNotBreakOthers(t *testing.T) {
	dir := projectDir(t)
	deps := testDeps(envtest.Processes{Err: errors.New("process table unreadable")})
	code, out, _ := runCLI(t, deps, "--path", dir, "--no-docker")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"Project", "Runtime", "Config", "could not list sockets: process table unreadable"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestNonTTYOutputIsPlain(t *testing.T) {
	dir := projectDir(t)
	_, out, _ := runCLI(t, testDeps(envtest.Processes{}), "--path", dir, "--no-docker")
	if strings.Contains(out, "\x1b[") {
		t.Error("ANSI escapes written to a non-terminal")
	}
}

func TestTextStyle(t *testing.T) {
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	cases := []struct {
		name      string
		tty       bool
		env       map[string]string
		noColor   bool
		wantColor bool
		wantEmoji bool
	}{
		{"pipe", false, nil, false, false, false},
		{"tty", true, nil, false, true, true},
		{"tty no-color flag", true, nil, true, false, true},
		{"tty NO_COLOR", true, map[string]string{"NO_COLOR": "1"}, false, false, true},
		{"tty empty NO_COLOR is ignored", true, map[string]string{"NO_COLOR": ""}, false, true, true},
		{"tty dumb term", true, map[string]string{"TERM": "dumb"}, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := textStyle(Deps{IsTerminal: tc.tty, LookupEnv: env(tc.env)}, tc.noColor)
			if st.Color != tc.wantColor || st.Emoji != tc.wantEmoji {
				t.Errorf("style = %+v", st)
			}
		})
	}
}

// Ensure the production dependency wiring builds without error.
func TestSystemDepsWire(t *testing.T) {
	d := SystemDeps(&bytes.Buffer{})
	if d.Registry == nil || d.Runner == nil || d.FS == nil || d.Processes == nil || d.Docker == nil {
		t.Fatalf("incomplete deps: %+v", d)
	}
	if d.IsTerminal {
		t.Error("a bytes.Buffer is never a terminal")
	}
}
