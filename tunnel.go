package mqtunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

type TunnelType string

const (
	TunnelTypeIn  TunnelType = "in"
	TunnelTypeOut TunnelType = "out"
)

type Tunnel struct {
	LocalPort   int    `json:"local_port"`
	LocalTopic  string `json:"local_topic"`
	RemotePort  int    `json:"remote_port"`
	RemoteTopic string `json:"remote_topic"`

	tunnelType     TunnelType
	conf           Config
	tcpConnection  *TCPConnection
	mqttConnection *mqttConnection
}

func NewTunnel(conf Config, local, remote int) Tunnel {
	// port topic should same level of control topic
	t := strings.Split(conf.Control, "/")
	root := strings.Join(t[:len(t)-1], "/")

	return Tunnel{
		LocalPort:   local,
		LocalTopic:  fmt.Sprintf("%s/%d", root, local),
		RemotePort:  remote,
		RemoteTopic: fmt.Sprintf("%s/%d", root, remote),

		tunnelType: TunnelTypeIn,
		conf:       conf,
	}
}

func (tun *MQTunnel) OpenTunnel(ctx context.Context, tunnel Tunnel) error {
	tunnel.mqttConnection = tun.mqtt

	tcon, err := NewTCPConnection(tun.conf, tunnel.LocalPort, tunnel)
	if err != nil {
		return fmt.Errorf("open tcp connection error, %w", err)
	}
	tunnel.tcpConnection = tcon

	go tcon.Start(ctx)

	if err := tun.mqtt.OpenTunnel(tunnel.RemoteTopic, tunnel); err != nil {
		return fmt.Errorf("broker open error, %w", err)
	}
	// TODO: where should we unsubscribe?
	// defer tun.mqtt.client.Unsubscribe(tunnel.RemoteTopic)

	return nil
}

// Write writes a payload from MQTT Broker to local connection
func (tun *Tunnel) Write(b []byte) (int, error) {
	zap.S().Debugw("Write", zap.String("topic", tun.RemoteTopic), zap.String("payload", string(b)))
	return tun.tcpConnection.write(b)
}

// Publish publish a payload to MQTT Broker
func (tun *Tunnel) Publish(b []byte) (int, error) {
	// does not wait publish
	zap.S().Debugw("Publish", zap.String("topic", tun.LocalTopic), zap.String("payload", string(b)))
	tun.mqttConnection.Publish(tun.LocalTopic, 0, false, b)
	return len(b), nil
}

func NewTunnelFromMsg(conf Config, msg mqtt.Message) (Tunnel, error) {
	var ret Tunnel

	if err := json.Unmarshal(msg.Payload(), &ret); err != nil {
		return ret, fmt.Errorf("open request unmarshal error, %w", err)
	}

	ret.conf = conf // set from local mqtunnel
	// swap local and remote
	ret.LocalPort, ret.RemotePort = ret.RemotePort, ret.LocalPort
	ret.LocalTopic, ret.RemoteTopic = ret.RemoteTopic, ret.LocalTopic
	ret.tunnelType = TunnelTypeOut

	return ret, nil
}
