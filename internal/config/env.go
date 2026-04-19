package config

import (
	"os"
	"strconv"
)

func Load() Config {
	cfg := Default()

	if v := os.Getenv("SOBS_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := os.Getenv("SOBS_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("SOBS_TEMPLATE_ROOT"); v != "" {
		cfg.TemplateRoot = v
	}
	if v := os.Getenv("SOBS_SESSION_COOKIE_NAME"); v != "" {
		cfg.SessionCookieName = v
	}
	if v := os.Getenv("SOBS_TRUSTED_PROXY_MODE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.TrustedProxyMode = b
		}
	}
	if v := os.Getenv("SOBS_ENFORCE_API_AUTH"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.EnforceAPIAuth = b
		}
	}

	return cfg
}
