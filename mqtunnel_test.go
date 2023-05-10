package mqtunnel

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func NewMockMQTunnel(t *testing.T) (*MQTunnel, string) {
	t.Helper()
	clientID := randStr(10)
	conf := Config{
		Host:     "localhost",
		Port:     1883,
		ClientID: clientID,
		Control:  fmt.Sprintf("d/%s/control", clientID),
	}
	mqt, err := NewMQTunnel(conf)
	assert.Nil(t, err)
	assert.NotNil(t, mqt)

	return mqt, fmt.Sprintf("d/%s/", clientID)
}

func TestMQTunnelhandleControl(t *testing.T) {
	t.Run("Request", func(t *testing.T) {
		mqt, root := NewMockMQTunnel(t)
		mqt.isLocal = true
		ctx := context.Background()
		ctl := controlPacket{
			Type:        controlTypeConnectRequest,
			TunnelID:    "1",
			RemotePort:  1883,
			RemoteTopic: fmt.Sprintf("%s/1-a", root),
			LocalPort:   2883,
			LocalTopic:  fmt.Sprintf("%s/1-b", root),
		}
		err := mqt.handleControl(ctx, ctl)
		assert.Nil(t, err)
	})
}
