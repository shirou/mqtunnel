package mqtunnel

import "math/rand"

type controlType string

const (
	controlTypeConnectRequest   controlType = "connect"
	controlTypeConnectAck       controlType = "connect_ack"
	controlTypeConnectionClosed controlType = "closed"
)

type controlPacket struct {
	Type     controlType `json:"type"`
	TunnelID string

	LocalPort   int    `json:"local_port"`
	LocalTopic  string `json:"local_topic"`
	RemotePort  int    `json:"remote_port"`
	RemoteTopic string `json:"remote_topic"`
}

const randomLetters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randStr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = randomLetters[rand.Intn(len(randomLetters))]
	}
	return string(b)
}
