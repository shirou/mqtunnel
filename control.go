package mqtunnel

import "math/rand"

type ControlType string

const (
	ControlTypeConnectRequest   ControlType = "connect"
	ControlTypeConnectAck       ControlType = "connect_ack"
	ControlTypeConnectionClosed ControlType = "closed"
)

type ControlPacket struct {
	Type     ControlType `json:"type"`
	TunnelID string

	LocalPort   int    `json:"local_port"`
	LocalTopic  string `json:"local_topic"`
	RemotePort  int    `json:"remote_port"`
	RemoteTopic string `json:"remote_topic"`

	AckTopic string `json:"ack_topic"`
}

const randomLetters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randStr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = randomLetters[rand.Intn(len(randomLetters))]
	}
	return string(b)
}
