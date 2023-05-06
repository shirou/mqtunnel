package mqtunnel

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"go.uber.org/zap"
)

const tunnelQoS = 0

// MQTunnel is a main component of mqtunnel.
type MQTunnel struct {
	conf       Config
	mqttBroker *mqttBroker

	controlCh chan ControlPacket
	localCh   chan net.Conn

	ackWaiting map[string]*Tunnel
	connected  map[string]*Tunnel

	isLocal bool

	mu sync.Mutex
}

func NewMQTunnel(conf Config) (*MQTunnel, error) {
	ret := MQTunnel{
		conf: conf,

		controlCh: make(chan ControlPacket),
		localCh:   make(chan net.Conn),

		ackWaiting: make(map[string]*Tunnel),
		connected:  make(map[string]*Tunnel),
	}

	mqBroker, err := NewMQTTBroker(conf, ret.controlCh)
	if err != nil {
		return nil, fmt.Errorf("MQTT connection error, %w", err)
	}
	ret.mqttBroker = mqBroker

	return &ret, nil
}

// Start starts MQTT broker connection, and also start waiting TCP connection if
// this is a local mqtunnel.
func (mqt *MQTunnel) Start(ctx context.Context, localPort, remotePort int) error {
	go mqt.mqttBroker.Start(ctx)

	if localPort != 0 && remotePort != 0 {
		zap.S().Debugw("this is local side")
		listener, err := NewTCPListener(mqt.conf, localPort)
		if err != nil {
			return fmt.Errorf("new TCPListener failed, %w", err)
		}
		go listener.StartListening(ctx, mqt.localCh)
		mqt.isLocal = true
	}

	for {
		select {
		case ctl := <-mqt.mqttBroker.controlCh:
			zap.S().Debugw("control",
				zap.String("type", string(ctl.Type)),
				zap.String("ID", ctl.TunnelID),
				zap.Bool("isLocal", mqt.isLocal))

			switch ctl.Type {
			case ControlTypeConnectRequest:
				if mqt.isLocal { // remote only
					continue
				}
				tun, err := NewTunnelFromControl(ctx, mqt.mqttBroker, ctl)
				if err != nil {
					zap.S().Errorw("NewTunnelFromControl failed", zap.Error(err))
					continue
				}
				if err := tun.setupRemoteTunnel(ctx); err != nil {
					zap.S().Errorw("setupRemoteTunnel failed", zap.Error(err))
					continue
				}
				go tun.MainLoop(ctx)
			case ControlTypeConnectAck:
				if !mqt.isLocal { // local only
					continue
				}
				tun, exists := mqt.ackWaiting[ctl.TunnelID]
				if exists {
					go tun.MainLoop(ctx)
					mqt.mu.Lock()
					delete(mqt.ackWaiting, ctl.TunnelID)
					mqt.connected[ctl.TunnelID] = tun
					mqt.mu.Unlock()
				}
			case ControlTypeConnectionClosed:
				tun, exists := mqt.connected[ctl.TunnelID]
				if exists {
					tun.cancel()
					delete(mqt.connected, ctl.TunnelID)
				}
			default:
				return fmt.Errorf("unknown control type, %s", ctl.Type)
			}

		case conn := <-mqt.localCh:
			// topics should be same level as control topic
			t := strings.Split(mqt.conf.Control, "/")
			root := strings.Join(t[:len(t)-1], "/")

			tun, err := NewTunnelFromConnect(ctx, mqt.mqttBroker, conn, root, localPort, remotePort)
			if err != nil {
				zap.S().Errorw("NewTunneclFromConnect failed", zap.Error(err))
				continue
			}
			if err := tun.setupLocalTunnel(ctx); err != nil {
				zap.S().Errorw("setupLocalTunnel failed", zap.Error(err))
				continue
			}
			if err := tun.OpenRequest(ctx); err != nil {
				zap.S().Errorw("OpenRequest failed", zap.Error(err))
				continue
			}

			mqt.ackWaiting[tun.ID] = tun

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
