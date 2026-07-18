// virtui/internal/config/config.go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	MaxLogLines int    `json:"max_log_lines"`
	IPv4Only    bool   `json:"ipv4_only"`
	LibvirtDir  string `json:"libvirt_dir"`
}

func Default() *Config {
	return &Config{
		MaxLogLines: 50,
		IPv4Only:    false,
		LibvirtDir:  "/var/lib/libvirt/",
	}
}

func Load() *Config {
	cfg := Default()

	dir := filepath.Join(os.Getenv("HOME"), ".local", "virtui")
	path := filepath.Join(dir, "config")

	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, cfg)
	}

	if _, err := os.Stat("/home/libvirt/images"); err == nil {
		cfg.LibvirtDir = "/home/libvirt/"
	}

	return cfg
}

