package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type ConvertConfig struct {
	DataDir string `yaml:"data_dir"`
	OutFile string `yaml:"out_file"`
	Pattern string `yaml:"pattern"`
}

type SearchConfig struct {
	DefaultLimit  int `yaml:"default_limit"`
	MaxLimit      int `yaml:"max_limit"`
	DefaultOffset int `yaml:"default_offset"`
}

// Config holds all configuration for the application
type Config struct {
	Port         string        `yaml:"port"`
	DataDir      string        `yaml:"data_dir"`
	Convert      ConvertConfig `yaml:"convert"`
	Search       SearchConfig  `yaml:"search"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
	NumWorkers   int           `yaml:"num_workers"`
}

// Load reads configuration from a YAML file
func Load(configPath string) (*Config, error) {
	cfg := &Config{
		Port:    "8080",
		DataDir: "../data",
		Convert: ConvertConfig{
			DataDir: "data",
			OutFile: "data/records.json",
			Pattern: "data/File *",
		},
		Search: SearchConfig{
			DefaultLimit:  20,
			MaxLimit:      100,
			DefaultOffset: 0,
		},
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
