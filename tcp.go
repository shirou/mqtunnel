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
	isLocal   bool
	tunnel    *Tunnel
}

func NewTCPConnection(conf Config, port int, tun *Tunnel) (*TCPConnection, error) {
	ret := TCPConnection{
		conf:   conf,
		port:   port,
		tunnel: tun,
	}

	return &ret, nil
}

func (con *TCPConnection) StartListening(ctx context.Context) error {
	go con.listen(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (con *TCPConnection) listen(ctx context.Context) error {
	addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf(":%d", con.port))
	if err != nil {
		zap.S().Errorw("ResolveTCPAddr error", zap.Error(err))
		return fmt.Errorf("ResolveTCPAddr error, %w", err)
	}

	listener, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		zap.S().Errorw("listen error", zap.Error(err))
		return fmt.Errorf("listen error, %w", err)
	}
	defer listener.Close()
	zap.S().Infow("start listening", zap.Int("port", con.port))

	// create a child context
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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
		zap.S().Debugw("accepted", zap.Int("local_port", con.port), zap.String("remote", conn.RemoteAddr().String()))

		// send open request to remote side
		if err := con.tunnel.OpenRequest(ctx); err != nil {
			return fmt.Errorf("openRequest failed, %w", err)
		}

		// Block other connections to prevent line congestion.
		if err := con.handleRead(ctx, conn); err != nil {
			return err
		}
	}
}

// Connect connects to TCP port on remote mqtunnel side.
func (con *TCPConnection) Connect(ctx context.Context) (net.Conn, error) {
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
	con.writeConn = conn
	zap.S().Debugw("connected", zap.Int("local_port", con.tunnel.LocalPort))
	go con.handleRead(ctx, conn)
	return conn, nil
}

// handleWrite writes to conn
func (con *TCPConnection) handleWrite(ctx context.Context, b []byte) (int, error) {
	zap.S().Debugw("tcp write", zap.Int("port", con.port), zap.Int("size", len(b)))

	if con.conn == nil {
		return 0, fmt.Errorf("write but not connected yet")
	}

	return con.conn.Write(b)
}

func (con *TCPConnection) handleRead(ctx context.Context, conn net.Conn) error {
	zap.S().Debugw("handleRead", zap.Int("port", con.port))

	defer conn.Close()
	for {
		buf := make([]byte, bufferSize)
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				zap.S().Error("tcp read error", zap.Error(err))
			}
			return err
		}
		zap.S().Debugw("handleReader", zap.Int("size", n))
		con.tunnel.publishCh <- buf[:n]
	}
}
