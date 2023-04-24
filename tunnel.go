package mqtunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

type TunnelType string

const (
	TunnelTypeLocal  TunnelType = "local"
	TunnelTypeRemote TunnelType = "remote"
)

type Tunnel struct {
	LocalPort   int    `json:"local_port"`
	LocalTopic  string `json:"local_topic"`
	RemotePort  int    `json:"remote_port"`
	RemoteTopic string `json:"remote_topic"`

	AckTopic string `json:"ack_topic"`

	tunnelType    TunnelType
	conf          Config
	tcpConnection *TCPConnection
	mqttBroker    *mqttBroker

	ctx    context.Context
	cancel context.CancelFunc

	writeCh   chan []byte // writeCh writes a payload from MQTT Broker to local connection
	publishCh chan []byte // publishCh publish a payload to MQTT Broker
}

func NewTunnel(conf Config, local, remote int) (*Tunnel, error) {
	// port topic should same level of control topic
	t := strings.Split(conf.Control, "/")
	root := strings.Join(t[:len(t)-1], "/")

	ctx, cancel := context.WithCancel(context.Background())

	ret := &Tunnel{
		ctx:         ctx,
		cancel:      cancel,
		LocalPort:   local,
		LocalTopic:  fmt.Sprintf("%s/%d-%s", root, local, randStr()),
		RemotePort:  remote,
		RemoteTopic: fmt.Sprintf("%s/%d-%s", root, remote, randStr()),

		AckTopic: fmt.Sprintf("%s/%d-%s", root, remote, randStr()+"-ack"),

		tunnelType: TunnelTypeLocal,
		conf:       conf,
		writeCh:    make(chan []byte, bufferSize*10),
		publishCh:  make(chan []byte, bufferSize*10),
	}

	tcon, err := NewTCPConnection(conf, ret.LocalPort, ret)
	if err != nil {
		return nil, fmt.Errorf("new tcp connection error, %w", err)
	}
	ret.tcpConnection = tcon

	return ret, nil
}

// NewTunnelFromMsg creates a new Tunnel on remote side from local.
func NewTunnelFromMsg(conf Config, msg mqtt.Message, mqttBroker *mqttBroker) (*Tunnel, error) {
	var ret Tunnel

	if err := json.Unmarshal(msg.Payload(), &ret); err != nil {
		return nil, fmt.Errorf("open request unmarshal error, %w", err)
	}

	ret.conf = conf // set from remote mqtunnel conf
	ret.tunnelType = TunnelTypeRemote
	// swap local and remote
	ret.LocalPort, ret.RemotePort = ret.RemotePort, ret.LocalPort
	ret.LocalTopic, ret.RemoteTopic = ret.RemoteTopic, ret.LocalTopic
	ret.writeCh = make(chan []byte, bufferSize*10)
	ret.publishCh = make(chan []byte, bufferSize*10)
	ret.mqttBroker = mqttBroker

	// create a new context
	ctx, cancel := context.WithCancel(context.Background())
	ret.ctx = ctx
	ret.cancel = cancel

	tcon, err := NewTCPConnection(conf, ret.LocalPort, &ret)
	if err != nil {
		return nil, fmt.Errorf("new tcp connection error, %w", err)
	}
	ret.tcpConnection = tcon

	return &ret, nil
}

// setupLocalTunnel opens
func (tun *Tunnel) setupLocalTunnel(ctx context.Context) error {
	// local side subcscribe ack topic to wait connect ack.
	if err := tun.mqttBroker.SubscribeControl(tun.AckTopic); err != nil {
		return fmt.Errorf("failed to control subscribe, %w", err)
	}

	if err := tun.mqttBroker.SubscribeTunnelTopic(tun.RemoteTopic, tun); err != nil {
		return fmt.Errorf("broker open error, %w", err)
	}

	go tun.tcpConnection.StartListening(ctx)

	return nil
}

// setupRemoteTunnel opens on remote
func (tun *Tunnel) setupRemoteTunnel(ctx context.Context) error {
	if _, err := tun.tcpConnection.Connect(ctx); err != nil {
		return fmt.Errorf("tcp connection error, %w", err)
	}

	if err := tun.mqttBroker.SubscribeTunnelTopic(tun.RemoteTopic, tun); err != nil {
		return fmt.Errorf("broker open error, %w", err)
	}

	token := tun.mqttBroker.Publish(ctx, tun.AckTopic, 1, false, "ack")
	token.Wait()

	return token.Error()
}

// OpenRequest sends a control packet to remote side
func (tun *Tunnel) OpenRequest(ctx context.Context) error {
	buf, _ := json.Marshal(tun)
	token := tun.mqttBroker.Publish(ctx, tun.conf.Control, 1, false, buf)
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("publish control msg error, %w", err)
	}

	return nil
}

func (tun *Tunnel) MainLoop(ctx context.Context) {
	defer tun.mqttBroker.Unsubscribe(tun.RemoteTopic)

	zap.S().Infow("start MainLoop")
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
		case b := <-tun.publishCh:
			// publish does not wait
			zap.S().Debugw("publishCh",
				zap.String("remote_topic", tun.LocalTopic),
				zap.Int("size", len(b)))
			tun.mqttBroker.Publish(ctx, tun.LocalTopic, 0, false, b)
		case <-ctx.Done():
			return
		}
	}
}

const randomLetters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randStr() string {
	n := 8
	b := make([]byte, n)
	for i := range b {
		b[i] = randomLetters[rand.Intn(len(randomLetters))]
	}
	return string(b)
}
