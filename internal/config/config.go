package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr               string
	DataDir            string
	MountRoot          string
	CookieSecure       bool
	RequireHTTPS       bool
	WebDAVRequireHTTPS bool
	TrustedProxyNets   []*net.IPNet
	SessionTTL         time.Duration
	MaxUploadBytes     int64
	TLSCertFile        string
	TLSKeyFile         string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:               envOr("APP_ADDR", ":8080"),
		DataDir:            envOr("APP_DATA_DIR", "./data"),
		SessionTTL:         7 * 24 * time.Hour,
		CookieSecure:       false,
		RequireHTTPS:       false,
		WebDAVRequireHTTPS: true,
		MaxUploadBytes:     20 << 30,
		TLSCertFile:        strings.TrimSpace(os.Getenv("APP_TLS_CERT_FILE")),
		TLSKeyFile:         strings.TrimSpace(os.Getenv("APP_TLS_KEY_FILE")),
	}
	if runtime.GOOS == "windows" {
		cfg.MountRoot = ""
	} else {
		cfg.MountRoot = envOr("APP_MOUNT_ROOT", "/mnt/personalcloud")
	}

	if raw := strings.TrimSpace(os.Getenv("APP_COOKIE_SECURE")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("APP_COOKIE_SECURE: %w", err)
		}
		cfg.CookieSecure = value
	}

	if raw := strings.TrimSpace(os.Getenv("APP_REQUIRE_HTTPS")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("APP_REQUIRE_HTTPS: %w", err)
		}
		cfg.RequireHTTPS = value
	}
	if raw := strings.TrimSpace(os.Getenv("APP_WEBDAV_REQUIRE_HTTPS")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("APP_WEBDAV_REQUIRE_HTTPS: %w", err)
		}
		cfg.WebDAVRequireHTTPS = value
	}
	if raw := strings.TrimSpace(os.Getenv("APP_MAX_UPLOAD_BYTES")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1<<20 {
			return Config{}, fmt.Errorf("APP_MAX_UPLOAD_BYTES debe ser un entero de al menos 1048576")
		}
		cfg.MaxUploadBytes = value
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

	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return Config{}, fmt.Errorf("APP_TLS_CERT_FILE y APP_TLS_KEY_FILE deben configurarse juntos")
	}
	if cfg.RequireHTTPS || cfg.TLSCertFile != "" {
		cfg.CookieSecure = true
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
