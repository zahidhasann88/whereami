package system

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOSRunnerMissingBinary(t *testing.T) {
	_, err := OSRunner{}.Run(context.Background(), t.TempDir(), "whereami-no-such-binary-xyz")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("err = %v, want exec.ErrNotFound", err)
	}
}

func TestOSRunnerExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell")
	}
	_, err := OSRunner{}.Run(context.Background(), t.TempDir(), "sh", "-c", "echo oops >&2; exit 3")
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 3 {
		t.Fatalf("err = %v, want exit code 3", err)
	}
}

func TestOSRunnerCapturesStreamsAndLocale(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell")
	}
	out, err := OSRunner{}.Run(context.Background(), t.TempDir(), "sh", "-c", "echo out; echo err >&2; echo $LC_ALL")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out.Stdout)) != "out\nC" {
		t.Errorf("stdout = %q", out.Stdout)
	}
	if strings.TrimSpace(string(out.Stderr)) != "err" {
		t.Errorf("stderr = %q", out.Stderr)
	}
}

func TestDockerEndpointResolution(t *testing.T) {
	if got := dockerEndpoint("unix:///run/user/1000/docker.sock"); got != "/run/user/1000/docker.sock" {
		t.Errorf("unix host = %q", got)
	}
	if runtime.GOOS == "windows" {
		if got := dockerEndpoint("npipe:////./pipe/docker_engine"); got != `\\.\pipe\docker_engine` {
			t.Errorf("npipe host = %q", got)
		}
	}
	if got := dockerEndpoint(""); got == "" {
		t.Error("default endpoint empty")
	}
}

// TestDockerClientOverUnixSocket serves a fake Engine API on a unix socket.
func TestDockerClientOverUnixSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets are not used on Windows")
	}
	dir, err := os.MkdirTemp("", "wh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("cannot listen on unix socket: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "bad method", http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`[{"Id":"abc","Names":["/web"]}]`))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	client := &DockerClient{endpoint: sock}
	client.client = newHTTPClient(sock)
	body, err := client.Get(context.Background(), "/containers/json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"/web"`) {
		t.Errorf("body = %s", body)
	}
	if _, err := client.Get(context.Background(), "/missing"); err == nil {
		t.Error("expected HTTP 404 to be an error")
	}

	// A missing socket is an error, not a panic.
	gone := &DockerClient{endpoint: filepath.Join(dir, "absent.sock")}
	gone.client = newHTTPClient(gone.endpoint)
	if _, err := gone.Get(context.Background(), "/containers/json"); err == nil {
		t.Error("expected error for missing socket")
	}
}

// TestGopsutilListenersFindsOwnSocket checks listener ownership against this process.
func TestGopsutilListenersFindsOwnSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	listeners, err := GopsutilProcesses{}.Listeners(context.Background())
	if err != nil {
		t.Skipf("socket listing unavailable: %v", err)
	}
	var found bool
	for _, l := range listeners {
		if l.Port == port && l.PID == int32(os.Getpid()) {
			found = true
			if l.CwdErr != nil {
				t.Skipf("working directory unreadable here: %v", l.CwdErr)
			}
			wd, _ := os.Getwd()
			if l.Cwd != wd && !strings.EqualFold(filepath.Clean(l.Cwd), filepath.Clean(wd)) {
				t.Errorf("cwd = %q, want %q", l.Cwd, wd)
			}
		}
	}
	if !found {
		t.Errorf("port %d not reported among %d listeners", port, len(listeners))
	}
}
