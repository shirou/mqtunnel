package mqtunnel

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

const tunnelQoS = 0

// MQTunnel is a main component of mqtunnel.
type MQTunnel struct {
	conf       Config
	mqttBroker *mqttBroker

	openCh chan *Tunnel // open request comes from MQTTBroker
	ackCh  chan string  // ack comes from MQTT Broker
}

func NewMQTunnel(conf Config) (*MQTunnel, error) {
	ret := MQTunnel{
		conf: conf,

		openCh: make(chan *Tunnel),
		ackCh:  make(chan string),
	}

	return &ret, nil
}

// Start starts MQTT broker connection, and also start waiting TCP connection if
// this is a local mqtunnel.
func (mqt *MQTunnel) Start(ctx context.Context, tun *Tunnel) error {
	mqBroker, err := NewMQTTBroker(mqt.conf, mqt.openCh, mqt.ackCh)
	if err != nil {
		return fmt.Errorf("MQTT connection error, %w", err)
	}
	go mqBroker.Start(ctx)

	mqt.mqttBroker = mqBroker
	tun.mqttBroker = mqBroker

	if tun.LocalPort != 0 && tun.RemotePort != 0 {
		zap.S().Debugw("this is local side")

		if err := tun.setupLocalTunnel(ctx); err != nil {
			return fmt.Errorf("failed to open tunnel, %w", err)
		}
	} else {
		zap.S().Debugw("this is remote side")
		// subscribe the control topic
		if err := mqBroker.SubscribeControl(mqt.conf.Control); err != nil {
			return fmt.Errorf("failed to control subscribe, %w", err)
		}
	}

	for {
		select {
		case tun := <-mqt.openCh:
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			go tun.MainLoop(ctx)

		case <-mqt.ackCh:
			// receive ack on local side
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			go tun.MainLoop(ctx)

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
