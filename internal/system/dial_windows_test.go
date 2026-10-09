//go:build windows

package system

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

func TestDockerClientOverNamedPipe(t *testing.T) {
	path := fmt.Sprintf(`\\.\pipe\whereami-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	ln, err := winio.ListenPipe(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/containers/json" {
				http.Error(w, "unexpected request", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`[{"Id":"pipe-test"}]`))
		}),
	}
	go func() { _ = server.Serve(ln) }()
	t.Cleanup(func() { _ = server.Close(); _ = ln.Close() })
	client := &DockerClient{endpoint: path, client: newHTTPClient(path)}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	body, err := client.Get(ctx, "/containers/json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "pipe-test") {
		t.Fatalf("unexpected response: %s", body)
	}
}

func TestNamedPipeDialCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn, err := dialSocket(ctx, `\\.\pipe\whereami-missing-pipe`)
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("expected canceled dial to fail")
	}
}
