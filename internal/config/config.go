package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Interface               string            `yaml:"interface"`
	EBPFMapSize             int               `yaml:"ebpf_map_size"`
	DiscoveryInterval       time.Duration     `yaml:"discovery_interval"`
	MetricsInterval         time.Duration     `yaml:"metrics_interval"`
	PlayerActivityThreshold time.Duration     `yaml:"player_activity_threshold"`
	MinPacketsThreshold     uint64            `yaml:"min_packets_threshold"`
	MinBytesThreshold       uint64            `yaml:"min_bytes_threshold"`
	ServerAddr              string            `yaml:"server_addr"`
	APIKey                  string            `yaml:"api_key"`
	PrometheusAddr          string            `yaml:"prometheus_addr"`
	DockerLabels            map[string]string `yaml:"docker_labels"`
	ServerIDSource          string            `yaml:"server_id_source"`
	PortEnvVar              string            `yaml:"port_env_var"`
	LogLevel                string            `yaml:"log_level"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		Interface:               "eth0",
		EBPFMapSize:             100000,
		DiscoveryInterval:       30 * time.Second,
		MetricsInterval:         30 * time.Second,
		PlayerActivityThreshold: 5 * time.Minute,
		MinPacketsThreshold:     50,
		MinBytesThreshold:       1000,
		ServerAddr:              ":8080",
		DockerLabels:            make(map[string]string),
		ServerIDSource:          "hostname",
		PortEnvVar:              "",
		LogLevel:                "info",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
