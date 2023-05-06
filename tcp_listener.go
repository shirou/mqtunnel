package mqtunnel

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
)

type TCPListener struct {
	conf Config
	port int
}

func NewTCPListener(conf Config, port int) (*TCPListener, error) {
	ret := TCPListener{
		conf: conf,
		port: port,
	}

	return &ret, nil
}

func (tcl *TCPListener) StartListening(ctx context.Context, localCh chan net.Conn) error {
	go tcl.listen(ctx, localCh)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (tcl *TCPListener) listen(ctx context.Context, localCh chan net.Conn) error {
	addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf(":%d", tcl.port))
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
	zap.S().Infow("start listening", zap.Int("port", tcl.port))

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

		zap.S().Debugw("accepted", zap.Int("local_port", tcl.port), zap.String("app", conn.RemoteAddr().String()))
		localCh <- conn

		/*
			con.conn = conn

			// send open request to remote side
			if err := con.tunnel.OpenRequest(ctx); err != nil {
				return fmt.Errorf("openRequest failed, %w", err)
			}

			// Block other connections to prevent line congestion.
			if err := con.handleRead(ctx, conn); err != nil {
				return err
			}
		*/
	}
}
