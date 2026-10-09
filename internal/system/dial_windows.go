//go:build windows

package system

import (
	"context"
	"net"
	"os"
)

// pipeConn adapts a Windows named pipe to net.Conn; file deadlines are unsupported.
type pipeConn struct {
	*os.File
}

type pipeAddr struct{}

func (pipeAddr) Network() string { return "npipe" }
func (pipeAddr) String() string  { return "npipe" }

func (pipeConn) LocalAddr() net.Addr  { return pipeAddr{} }
func (pipeConn) RemoteAddr() net.Addr { return pipeAddr{} }

func dialSocket(_ context.Context, path string) (net.Conn, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return pipeConn{f}, nil
}
