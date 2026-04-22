package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the application
type Config struct {
	Port         string        `yaml:"port" default:"8080"`
	DataDir      string        `yaml:"data_dir" default:"../data"`
	ReadTimeout  time.Duration `yaml:"read_timeout" default:"10s"`
	WriteTimeout time.Duration `yaml:"write_timeout" default:"30s"`
	IdleTimeout  time.Duration `yaml:"idle_timeout" default:"60s"`
	NumWorkers   int           `yaml:"num_workers" default:"8"`
}

// Load reads configuration from a YAML file
func Load(configPath string) (*Config, error) {
	cfg := &Config{
		Port:         "8080",
		DataDir:      "../data",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		NumWorkers:   8,
	}

	if configPath == "" {
		configPath = "config.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// If config file doesn't exist, use defaults
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
