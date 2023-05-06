package mqtunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"go.uber.org/zap"
)

type Tunnel struct {
	ID          string
	LocalPort   int
	LocalTopic  string
	RemotePort  int
	RemoteTopic string

	tcpConnection *TCPConnection
	mqttBroker    *mqttBroker

	ctx    context.Context
	cancel context.CancelFunc

	writeCh     chan []byte // writeCh writes a payload from MQTT Broker to local connection
	publishCh   chan []byte // publishCh publish a payload to MQTT Broker
	tcpClosedCh chan error
}

// NewTunnelFromConnect creates a new Tunnel on local
func NewTunnelFromConnect(ctx context.Context, mqttBroker *mqttBroker, conn net.Conn, topicRoot string, local, remote int) (*Tunnel, error) {
	ctx, cancel := context.WithCancel(ctx)

	ret := &Tunnel{
		ctx:         ctx,
		cancel:      cancel,
		ID:          randStr(16),
		LocalPort:   local,
		LocalTopic:  fmt.Sprintf("%s/%d-%s", topicRoot, local, randStr(8)),
		RemotePort:  remote,
		RemoteTopic: fmt.Sprintf("%s/%d-%s", topicRoot, remote, randStr(8)),

		writeCh:     make(chan []byte, bufferSize*10),
		publishCh:   make(chan []byte, bufferSize*10),
		tcpClosedCh: make(chan error),
		mqttBroker:  mqttBroker,
	}

	tcon, err := NewTCPConnection(ret.LocalPort, ret)
	if err != nil {
		return nil, fmt.Errorf("new tcp connection error, %w", err)
	}
	tcon.conn = conn
	ret.tcpConnection = tcon
	go tcon.handleRead(ctx)

	return ret, nil
}

// NewTunnelFromControl creates a new Tunnel on remote side.
func NewTunnelFromControl(ctx context.Context, mqttBroker *mqttBroker, ctl ControlPacket) (*Tunnel, error) {

	// create a new child context
	ctx, cancel := context.WithCancel(ctx)

	ret := Tunnel{
		ID: ctl.TunnelID,

		ctx:    ctx,
		cancel: cancel,

		// swap local and remote topic
		LocalPort:   ctl.RemotePort,
		LocalTopic:  ctl.RemoteTopic,
		RemotePort:  ctl.LocalPort,
		RemoteTopic: ctl.LocalTopic,

		writeCh:     make(chan []byte, bufferSize*10),
		publishCh:   make(chan []byte, bufferSize*10),
		tcpClosedCh: make(chan error),
		mqttBroker:  mqttBroker,
	}

	tcon, err := NewTCPConnection(ret.LocalPort, &ret)
	if err != nil {
		return nil, fmt.Errorf("new tcp connection error, %w", err)
	}
	ret.tcpConnection = tcon

	return &ret, nil
}

// setupLocalTunnel opens
func (tun *Tunnel) setupLocalTunnel(ctx context.Context) error {
	if err := tun.mqttBroker.SubscribeTunnelTopic(tun.RemoteTopic, tun); err != nil {
		return fmt.Errorf("broker open error, %w", err)
	}
	return nil
}

// setupRemoteTunnel opens on remote
func (tun *Tunnel) setupRemoteTunnel(ctx context.Context) error {
	if err := tun.mqttBroker.SubscribeTunnelTopic(tun.RemoteTopic, tun); err != nil {
		return fmt.Errorf("broker open error, %w", err)
	}

	if _, err := tun.tcpConnection.connect(ctx); err != nil {
		return fmt.Errorf("tcp connection error, %w", err)
	}

	ack, _ := json.Marshal(tun.createAck())

	token := tun.mqttBroker.Publish(ctx, tun.mqttBroker.controlTopic, 1, false, ack)
	token.Wait()

	return token.Error()
}

// OpenRequest sends a control packet to remote side
func (tun *Tunnel) OpenRequest(ctx context.Context) error {

	ctl := tun.createConnectRequest()
	buf, _ := json.Marshal(ctl)
	token := tun.mqttBroker.Publish(ctx, tun.mqttBroker.controlTopic, 1, false, buf)
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("publish control msg error, %w", err)
	}

	return nil
}

func (tun *Tunnel) MainLoop(ctx context.Context) {
	defer tun.mqttBroker.Unsubscribe(tun.RemoteTopic)

	zap.S().Infow("start MainLoop", zap.String("ID", tun.ID))
	for {
		select {
		case b := <-tun.writeCh:
			zap.S().Debugw("writeCh",
				zap.String("remote_topic", tun.RemoteTopic),
				zap.Int("size", len(b)))
			_, err := tun.tcpConnection.handleWrite(ctx, b)
			if err != nil {
				zap.S().Error(err)
			}
		case b, ok := <-tun.publishCh:
			if !ok {
				c := tun.createConnectionClosed()
				zap.S().Debugw("connection closed",
					zap.String("remote_topic", tun.LocalTopic),
					zap.Int("size", len(b)))
				tun.mqttBroker.Publish(ctx, tun.mqttBroker.controlTopic, 0, false, c)
				if len(b) > 0 {
					// send last bytes
					tun.mqttBroker.Publish(ctx, tun.LocalTopic, 0, false, b)
				}
				return
			}
			zap.S().Debugw("publishCh",
				zap.String("remote_topic", tun.LocalTopic),
				zap.Int("size", len(b)))
			tun.mqttBroker.Publish(ctx, tun.LocalTopic, 0, false, b)
		case <-ctx.Done():
			return
		}
	}
}

func (tun *Tunnel) createConnectRequest() ControlPacket {
	ret := ControlPacket{
		Type:        ControlTypeConnectRequest,
		TunnelID:    tun.ID,
		LocalPort:   tun.LocalPort,
		LocalTopic:  tun.LocalTopic,
		RemotePort:  tun.RemotePort,
		RemoteTopic: tun.RemoteTopic,
	}
	return ret
}
func (tun *Tunnel) createAck() ControlPacket {
	ret := ControlPacket{
		Type:     ControlTypeConnectAck,
		TunnelID: tun.ID,
	}
	return ret
}
func (tun *Tunnel) createConnectionClosed() ControlPacket {
	ret := ControlPacket{
		Type:     ControlTypeConnectionClosed,
		TunnelID: tun.ID,
	}
	return ret
}
