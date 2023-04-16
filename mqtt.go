package mqtunnel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mqttConnection struct {
	client             mqtt.Client
	conf               Config
	mqtt_disconnect_ch chan bool
	controlTopic       string
	tunnelTopics       map[string]Tunnel
	openTunnel         chan Tunnel
}

func newMQTTConnection(conf Config, openTunnel chan Tunnel) (*mqttConnection, error) {
	ret := mqttConnection{
		conf:               conf,
		mqtt_disconnect_ch: make(chan bool),
		tunnelTopics:       make(map[string]Tunnel),
		openTunnel:         openTunnel,
	}

	opts, err := getMQTTOptions(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to get MQTT options, %w", err)
	}

	// add callback
	opts.SetConnectionLostHandler(ret.onMqttConnectionLost)
	opts.SetOnConnectHandler(ret.onConnect)
	opts.SetReconnectingHandler(ret.onReconnect)

	mqtt.ERROR, _ = zap.NewStdLogAt(zap.L(), zap.ErrorLevel)
	mqtt.CRITICAL, _ = zap.NewStdLogAt(zap.L(), zap.ErrorLevel)
	// mqtt.WARN, _ = zap.NewStdLogAt(zap.L(), zap.WarnLevel)
	// mqtt.DEBUG, _ = zap.NewStdLogAt(logger.Desugar(), zap.DebugLevel)

	// connect to MQTT Broker
	client := mqtt.NewClient(opts)
	ret.client = client

	// connect first time
	if err := ret.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect broker, %w", err)
	}

	return &ret, nil
}

func (con *mqttConnection) Start(ctx context.Context) error {
	for {
		select {
		case <-con.mqtt_disconnect_ch:
			zap.S().Error("mqtt disconnect message. try to reconnect")
			// do nothing. auto-reconnect should work
		case <-ctx.Done():
			zap.S().Warnf("done: %v", ctx.Err())
			return ctx.Err()
		}
	}
}

func (con *mqttConnection) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	zap.S().Debugw("publish", zap.String("topic", topic))

	return con.client.Publish(topic, qos, retained, payload)
}

func (con *mqttConnection) connect() error {
	zap.S().Debugf("connect start")
	token := con.client.Connect()
	token.Wait()
	return token.Error()
}

func (con *mqttConnection) OpenTunnel(topic string, tunnel Tunnel) error {
	con.tunnelTopics[topic] = tunnel

	return con.subscribe()
}

func (con *mqttConnection) SubscribeControl(topic string) error {
	con.controlTopic = topic // does not need on in-side
	return con.subscribe()
}

func (con *mqttConnection) subscribe() error {
	topics := make(map[string]byte)

	if con.controlTopic != "" {
		topics[con.controlTopic] = 1
	}
	for t, _ := range con.tunnelTopics {
		topics[t] = tunnelQoS
	}

	if len(topics) == 0 {
		return nil
	}

	zap.S().Infow("topic subscribing", zap.Strings("topic", logTopic(topics)))

	subscribeToken := con.client.SubscribeMultiple(topics, con.onMessage)
	subscribeToken.Wait()
	return subscribeToken.Error()
}

func (con *mqttConnection) onMessage(client mqtt.Client, msg mqtt.Message) {
	zap.S().Debugw("on message", zap.String("topic", msg.Topic()))

	if msg.Topic() == con.conf.Control {
		// This is control message. start a new tunnel
		// But remote and local should be swapped
		tun, err := NewTunnelFromMsg(con.conf, msg)
		if err != nil {
			zap.S().Error("new tunnel request invalid, %w", err)
		} else {
			zap.S().Debugw("tunnel requested",
				zap.Int("local_port", tun.LocalPort),
				zap.String("local_topic", tun.LocalTopic),
				zap.Int("remote_port", tun.RemotePort),
				zap.String("remote_topic", tun.RemoteTopic),
			)
			con.openTunnel <- tun
		}

		return
	}
	tun, exists := con.tunnelTopics[msg.Topic()]
	if !exists {
		zap.S().Errorw("requested topic is not exists",
			zap.String("topic", msg.Topic()))
		return
	}
	tun.Write(msg.Payload())
}

func (con *mqttConnection) onConnect(client mqtt.Client) {
	zap.S().Info("connected")
	if err := con.subscribe(); err != nil {
		zap.S().Errorw("subscribe failed", zap.Error(err))
	}
}

func (con *mqttConnection) onReconnect(client mqtt.Client, opts *mqtt.ClientOptions) {
	zap.S().Info("reconnecting...")
}

func (con *mqttConnection) onMqttConnectionLost(client mqtt.Client, err error) {
	zap.S().Error("MQTT connection lost", zap.Error(err))
	con.mqtt_disconnect_ch <- true
}

func newTLSConfig(config Config) (*tls.Config, error) {
	rootCA, err := os.ReadFile(config.CaCert)
	if err != nil {
		return nil, err
	}
	certpool := x509.NewCertPool()
	certpool.AppendCertsFromPEM(rootCA)
	cert, err := tls.LoadX509KeyPair(config.ClientCert, config.PrivateKey)
	if err != nil {
		return nil, err
	}
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		RootCAs:            certpool,
		InsecureSkipVerify: true,
		ClientAuth:         tls.NoClientCert,
		ClientCAs:          nil,
		Certificates:       []tls.Certificate{cert},
		NextProtos:         []string{"x-amzn-mqtt-ca"},
	}, nil
}

func getMQTTOptions(conf Config) (*mqtt.ClientOptions, error) {
	u, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to make uuid, %w", err)
	}

	opts := mqtt.NewClientOptions()

	if conf.Port == 1883 {
		opts.AddBroker(fmt.Sprintf("tcp://%s:%d", conf.Host, conf.Port))
	} else {
		opts.AddBroker(fmt.Sprintf("ssl://%s:%d", conf.Host, conf.Port))
		tlsConfig, err := newTLSConfig(conf)
		if err != nil {
			return nil, fmt.Errorf("failed to construct tls config, %v", err)
		}
		opts.SetTLSConfig(tlsConfig)
	}
	opts.SetClientID(u.String())
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetryInterval(20 * time.Second)

	return opts, nil
}

// logTopic is a util function to log multiple topics
func logTopic(topics map[string]byte) []string {
	ret := make([]string, 0, len(topics))
	for k := range topics {
		ret = append(ret, k)
	}

	return ret
}
