package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	MaxLogLines int  `json:"max_log_lines"`
	IPv4Only    bool `json:"ipv4_only"`
}

func Default() *Config {
	return &Config{
		MaxLogLines: 50,
		IPv4Only:    false,
	}
}

func Load() *Config {
	cfg := Default()

	dir := filepath.Join(os.Getenv("HOME"), ".local", "virtui")
	path := filepath.Join(dir, "config")

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return cfg
	}

	return cfg
}
