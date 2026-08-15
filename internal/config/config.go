package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr             string
	DataDir          string
	CookieSecure     bool
	TrustedProxyNets []*net.IPNet
	SessionTTL       time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Addr:         envOr("APP_ADDR", ":8080"),
		DataDir:      envOr("APP_DATA_DIR", "./data"),
		SessionTTL:   7 * 24 * time.Hour,
		CookieSecure: false,
	}

	if raw := strings.TrimSpace(os.Getenv("APP_COOKIE_SECURE")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("APP_COOKIE_SECURE: %w", err)
		}
		cfg.CookieSecure = value
	}

	if raw := strings.TrimSpace(os.Getenv("APP_SESSION_TTL")); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("APP_SESSION_TTL: %w", err)
		}
		if value < time.Hour {
			return Config{}, fmt.Errorf("APP_SESSION_TTL debe ser al menos 1h")
		}
		cfg.SessionTTL = value
	}

	trusted, err := parseCIDRs(envOr("APP_TRUSTED_PROXIES", "127.0.0.1/32,::1/128"))
	if err != nil {
		return Config{}, fmt.Errorf("APP_TRUSTED_PROXIES: %w", err)
	}
	cfg.TrustedProxyNets = trusted

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return Config{}, fmt.Errorf("crear data dir: %w", err)
	}
	return cfg, nil
}

func (c Config) StorePath() string {
	return filepath.Join(c.DataDir, "state.json")
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func parseCIDRs(raw string) ([]*net.IPNet, error) {
	var result []*net.IPNet
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		_, network, err := net.ParseCIDR(item)
		if err != nil {
			return nil, fmt.Errorf("CIDR %q inválido", item)
		}
		result = append(result, network)
	}
	return result, nil
}
