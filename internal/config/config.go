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
	URI         string `json:"-"`
}

func Default() *Config {
	return &Config{
		MaxLogLines: 50,
		IPv4Only:    false,
		LibvirtDir:  "/var/lib/libvirt/",
		URI:         "qemu:///system",
	}
}

func Load() *Config {
	cfg := Default()

	dir := filepath.Join(os.Getenv("HOME"), ".local", "virtui")
	path := filepath.Join(dir, "config")

	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, cfg)
	} else {
		_ = os.MkdirAll(dir, 0755)
		if out, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			_ = os.WriteFile(path, out, 0644)
		}
	}

	return cfg
}

