package system

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	defaultUnixSocket = "/var/run/docker.sock"
	defaultPipe       = `\\.\pipe\docker_engine`
	maxResponseBytes  = 8 << 20
)

// DockerClient queries the local Docker socket or named pipe using net/http.
type DockerClient struct {
	endpoint string
	client   *http.Client
}

// NewDockerClient uses DOCKER_HOST or the default local endpoint without connecting.
func NewDockerClient() *DockerClient {
	endpoint := dockerEndpoint(os.Getenv("DOCKER_HOST"))
	return &DockerClient{endpoint: endpoint, client: newHTTPClient(endpoint)}
}

// newHTTPClient dials the given socket or named pipe.
func newHTTPClient(endpoint string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialSocket(ctx, endpoint)
			},
			DisableKeepAlives: true,
		},
		Timeout: 5 * time.Second,
	}
}

// Endpoint returns the socket path or pipe name that will be dialed.
func (c *DockerClient) Endpoint() string { return c.endpoint }

func (c *DockerClient) Get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("building docker request: %w", err)
	}
	req.Header.Set("Connection", "close")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker unavailable at %s: %w", c.endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading docker response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker returned HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// dockerEndpoint resolves DOCKER_HOST into a local socket path or pipe name.
func dockerEndpoint(dockerHost string) string {
	def := defaultUnixSocket
	if runtime.GOOS == "windows" {
		def = defaultPipe
	}
	switch {
	case strings.HasPrefix(dockerHost, "unix://"):
		return strings.TrimPrefix(dockerHost, "unix://")
	case strings.HasPrefix(dockerHost, "npipe://"):
		p := strings.TrimPrefix(dockerHost, "npipe://")
		return strings.ReplaceAll(p, "/", `\`)
	case dockerHost != "":
		// Unsupported Docker schemes fail as unavailable.
		return dockerHost
	}
	return def
}
