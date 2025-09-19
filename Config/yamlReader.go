package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Backend struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

type RateLimitConfig struct {
	Rate     int `yaml:"rate"`
	Capacity int `yaml:"capacity"`
}

type Config struct {
	Backends        []Backend     `yaml:"backends"`
	HealthCheckPath string        `yaml:"health_check_path"`
	LatencyMS       int           `yaml:"latency_ms"`
	ErrorRate       float64       `yaml:"error_rate"`
	Strategy        string        `yaml:"strategy"`
	StickyCookie    string        `yaml:"sticky_cookie"`
	RequestTimeout  time.Duration `yaml:"request_timeout"`
	MaxRetries      int           `yaml:"max_retries"`

	RateLimit struct {
		PerClient  RateLimitConfig `yaml:"per_client"`
		PerBackend RateLimitConfig `yaml:"per_backend"`
	} `yaml:"rate_limit"`
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
