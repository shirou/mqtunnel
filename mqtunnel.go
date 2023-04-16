package mqtunnel

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

const tunnelQoS = 0

type MQTunnel struct {
	conf Config
	mqtt *mqttConnection

	openRequest chan Tunnel
}

func NewMQTunnel(conf Config) (*MQTunnel, error) {
	ret := MQTunnel{
		conf:        conf,
		openRequest: make(chan Tunnel),
	}

	return &ret, nil
}

func (tun *MQTunnel) Start(ctx context.Context, tunnel Tunnel) error {
	mqtt, err := newMQTTConnection(tun.conf, tun.openRequest)
	if err != nil {
		return fmt.Errorf("mqtt connection error, %w", err)
	}
	go mqtt.Start(ctx)

	tun.mqtt = mqtt

	if tunnel.LocalPort != 0 && tunnel.RemotePort != 0 {
		// this is connection starter side
		zap.S().Debugw("this is starter(out) side")
		buf, _ := json.Marshal(tunnel)
		token := tun.mqtt.Publish(tun.conf.Control, 1, false, buf)
		token.Wait()
		if err := token.Error(); err != nil {
			return fmt.Errorf("publish control msg error, %w", err)
		}

		if err := tun.OpenTunnel(ctx, tunnel); err != nil {
			return fmt.Errorf("failed to open tunnel, %w", err)
		}
	} else {
		// subscribe control topic
		if err := mqtt.SubscribeControl(tun.conf.Control); err != nil {
			return fmt.Errorf("failed to control subscribe, %w", err)
		}
	}

	for {
		select {
		case tunnel := <-tun.openRequest:
			if err := tun.OpenTunnel(ctx, tunnel); err != nil {
				zap.S().Errorf("open tunnel error, %w", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
