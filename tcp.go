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
	conf Config
	port int

	// reader *bufio.Reader // bytes from MQTT
	// writer *bufio.Writer // bytes to MQTT

	conn net.Conn
	//writeConn bufio.Writer
	writeConn net.Conn
	tunnel    Tunnel
}

func NewTCPConnection(conf Config, port int, tun Tunnel) (*TCPConnection, error) {
	ret := TCPConnection{
		conf:   conf,
		port:   port,
		tunnel: tun,
	}

	return &ret, nil
}

func (con *TCPConnection) Start(ctx context.Context) error {
	zap.S().Debugw("Tunnel type", zap.String("type", string(con.tunnel.tunnelType)))
	if con.tunnel.tunnelType == TunnelTypeIn {
		go con.listen(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (con *TCPConnection) connect(ctx context.Context) (net.Conn, error) {
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
	return conn, nil
}

func (con *TCPConnection) listen(ctx context.Context) error {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", con.port))
	if err != nil {
		zap.S().Errorw("ResolveTCPAddr error", zap.Error(err))
		return fmt.Errorf("ResolveTCPAddr error, %w", err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		zap.S().Errorw("listen error", zap.Error(err))
		return fmt.Errorf("listen error, %w", err)
	}
	defer listener.Close()
	zap.S().Infow("start listening", zap.Int("port", con.port))

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				continue
			} else {
				return fmt.Errorf("accept error, %w", err)
			}
		}
		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(time.Second * 60)
		con.conn = conn
		con.writeConn = conn
		go con.handleReader(ctx, conn)
	}
}

// write writes to conn
func (con *TCPConnection) write(ctx context.Context, b []byte) (int, error) {
	zap.S().Debugw("tcp write", zap.Int("port", con.port), zap.Int("size", len(b)))

	if con.conn == nil {
		// not connected yet
		zap.S().Debugw("start connecting", zap.Int("local_port", con.tunnel.LocalPort))
		conn, err := con.connect(ctx)
		if err != nil {
			zap.S().Error(err)
			return 0, fmt.Errorf("connect error, %w", err)
		}
		con.conn = conn
		con.writeConn = conn
		zap.S().Debugw("connected", zap.Int("local_port", con.tunnel.LocalPort))
		go con.handleReader(ctx, conn)
	}

	return con.conn.Write(b)
}

func (con *TCPConnection) handleReader(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	for {
		buf := make([]byte, bufferSize)
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				zap.S().Error("tcp read error", zap.Error(err))
			}
			return
		}
		zap.S().Debugw("handleReader", zap.Int("size", n))
		con.tunnel.publishCh <- buf[:n]
	}
}
