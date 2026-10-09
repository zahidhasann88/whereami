//go:build windows

package system

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func dialSocket(ctx context.Context, path string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, path)
}
