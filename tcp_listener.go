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

	closeCh chan error
}

func NewTCPListener(conf Config, port int) (*TCPListener, error) {
	ret := TCPListener{
		conf:    conf,
		port:    port,
		closeCh: make(chan error),
	}

	return &ret, nil
}

func (tcl *TCPListener) startListening(ctx context.Context, localCh chan net.Conn) error {
	go tcl.listen(ctx, localCh)

	for {
		select {
		case err := <-tcl.closeCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (tcl *TCPListener) listen(ctx context.Context, localCh chan net.Conn) error {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", tcl.port))
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

	zap.S().Infow("start listening", zap.Int("port", tcl.port))

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				continue
			} else {
				tcl.closeCh <- err
				return fmt.Errorf("accept error, %w", err)
			}
		}

		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(time.Second * 60)

		zap.S().Debugw("accepted",
			zap.Int("local_port", tcl.port),
			zap.String("app_port", conn.RemoteAddr().String()))

		localCh <- conn
	}
}
