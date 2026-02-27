package config

import (
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	Server  ServerConfig  `json:"server"`
	Logging LoggingConfig `json:"logging"`
}

type ServerConfig struct {
	Bind string     `json:"bind"`
	TCP  TCPConfig  `json:"tcp"`
	UDP  UDPConfig  `json:"udp"`
	HTTP HTTPConfig `json:"http"`
	MQTT MQTTConfig `json:"mqtt"`
}

type TCPConfig struct {
	Port       int      `json:"port"`
	TLSPort    int      `json:"-"`
	Delay      Duration `json:"delay"`
	CloseAfter Duration `json:"close_after"`
}

type UDPConfig struct {
	Port     int      `json:"port"`
	DropRate float64  `json:"drop_rate"`
	Delay    Duration `json:"delay"`
}

type HTTPConfig struct {
	Port         int      `json:"port"`
	TLSPort      int      `json:"-"`
	SlowResponse bool     `json:"slow_response"`
	SlowDuration Duration `json:"slow_duration"`
	ErrorCode    int      `json:"error_code"`
	Chunked      bool     `json:"chunked"`
}

type MQTTConfig struct {
	Port    int  `json:"port"`
	TLSPort int  `json:"-"`
	Retain  bool `json:"retain"`
}

type LoggingConfig struct {
	Level string `json:"level"`
}

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			d.Duration = 0
			return nil
		}
		dur, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		d.Duration = dur
		return nil
	}
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	d.Duration = time.Duration(n)
	return nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Bind: "0.0.0.0",
			TCP: TCPConfig{
				Port:    9000,
				TLSPort: 9443,
			},
			UDP: UDPConfig{
				Port: 9001,
			},
			HTTP: HTTPConfig{
				Port:    8080,
				TLSPort: 8443,
			},
			MQTT: MQTTConfig{
				Port:    1883,
				TLSPort: 8883,
			},
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}
}
