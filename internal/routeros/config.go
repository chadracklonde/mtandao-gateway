package routeros

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var ErrInvalidConfig = errors.New("invalid RouterOS configuration")

type Config struct {
	Address        string `json:"address"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// ReadConfig loads credentials from a private regular file. Symlinks and files
// accessible by group/other are rejected; secrets are never included in errors.
func ReadConfig(path string) (Config, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Config{}, fmt.Errorf("inspect RouterOS config: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Config{}, ErrInvalidConfig
	}
	if info.Mode().Perm()&0077 != 0 {
		return Config{}, errors.New("RouterOS config permissions must be 0600 or stricter")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read RouterOS config: %w", err)
	}
	var cfg Config
	decErr := json.Unmarshal(data, &cfg)
	if decErr != nil {
		return Config{}, ErrInvalidConfig
	}
	cfg.Address = strings.TrimSpace(cfg.Address)
	cfg.Username = strings.TrimSpace(cfg.Username)
	if cfg.Address == "" || cfg.Username == "" || cfg.Password == "" {
		return Config{}, ErrInvalidConfig
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = 8
	}
	if cfg.TimeoutSeconds < 1 || cfg.TimeoutSeconds > 60 {
		return Config{}, ErrInvalidConfig
	}
	if err := ValidateLocalAddress(cfg.Address); err != nil {
		return Config{}, ErrInvalidConfig
	}
	return cfg, nil
}

func (c Config) TLSConfig() TLSConfig {
	return TLSConfig{Address: c.Address, Username: c.Username, Password: c.Password, Timeout: time.Duration(c.TimeoutSeconds) * time.Second}
}
