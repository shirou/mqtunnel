package mqtunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"go.uber.org/zap"
)

const bufferSize = 4096
const tunnelKeepAlivePeriod = time.Second * 180

type TCPConnection struct {
	port int

	// reader *bufio.Reader // bytes from MQTT
	// writer *bufio.Writer // bytes to MQTT

	conn        net.Conn
	isLocal     bool
	tunnel      *Tunnel
	tcpClosedCh chan error
}

func NewTCPConnection(port int, tun *Tunnel) (*TCPConnection, error) {
	ret := TCPConnection{
		port:        port,
		tunnel:      tun,
		tcpClosedCh: tun.tcpClosedCh,
	}

	return &ret, nil
}

// connect connects to TCP port on remote mqtunnel side.
func (con *TCPConnection) connect(ctx context.Context) (net.Conn, error) {
	zap.S().Debugw("start connecting", zap.Int("local_port", con.tunnel.LocalPort))

	// TODO: Does we want to any hosts instead of localhost?
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", con.port))
	if err != nil {
		return nil, fmt.Errorf("ResolveTCPAddr error, %w", err)
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, err
	}
	conn.SetKeepAlive(true)
	conn.SetKeepAlivePeriod(tunnelKeepAlivePeriod)

	con.conn = conn
	zap.S().Debugw("connected", zap.Int("local_port", con.tunnel.LocalPort))
	go con.handleRead(ctx)
	return conn, nil
}

// handleWrite writes to conn
func (con *TCPConnection) handleWrite(ctx context.Context, b []byte) (int, error) {
	zap.S().Debugw("handleWrite", zap.Int("port", con.port), zap.Int("size", len(b)))

	if con.conn == nil {
		return 0, fmt.Errorf("write but not connected yet")
	}

	return con.conn.Write(b)
}

func (con *TCPConnection) handleRead(ctx context.Context) error {
	zap.S().Debugw("handleRead", zap.Int("port", con.port))

	defer con.conn.Close()
	for {
		buf := make([]byte, bufferSize)
		n, err := con.conn.Read(buf)
		if err != nil {
			if n > 0 {
				con.tunnel.publishCh <- buf[:n]
			}
			close(con.tunnel.publishCh)
			if err == io.EOF {
				return nil
			}
			zap.S().Error("tcp read error", zap.Error(err))
			return err
		}
		zap.S().Debugw("handleReader", zap.Int("size", n))
		con.tunnel.publishCh <- buf[:n]
	}
}
