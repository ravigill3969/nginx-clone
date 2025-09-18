package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Backend struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

type Config struct {
	Backends        []Backend `yaml:"backends"`
	HealthCheckPath string   `yaml:"health_check_path"`
	LatencyMS       int      `yaml:"latency_ms"`
	ErrorRate       float64  `yaml:"error_rate"`
	Strategy        string   `yaml:"strategy"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var newCfg Config
	if err := yaml.Unmarshal(data, &newCfg); err != nil {
		return nil, err
	}

	return &newCfg, nil
}
