//go:build !windows

package system

import (
	"context"
	"net"
	"time"
)

func dialSocket(ctx context.Context, path string) (net.Conn, error) {
	d := net.Dialer{Timeout: 1500 * time.Millisecond}
	return d.DialContext(ctx, "unix", path)
}
