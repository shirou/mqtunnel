package mqtunnel

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	UserName string `json:"username"`
	Password string `json:"password"`
	ClientID string `json:"clientId"`

	CaCert     string `json:"caCert"`
	ClientCert string `json:"clientCert"`
	PrivateKey string `json:"privateKey"`

	Control string `json:"control"`
}

func ReadConfig(filePath string) (Config, error) {
	var ret Config

	buf, err := os.ReadFile(filePath)
	if err != nil {
		return ret, fmt.Errorf("read config error, %w", err)
	}

	if err := json.Unmarshal(buf, &ret); err != nil {
		return ret, fmt.Errorf("read config marshal error, %w", err)
	}
	return ret, nil
}
