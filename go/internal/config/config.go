package config

import (
	"os"
	"strconv"
	"strings"
)

// Cfg holds all runtime configuration parsed from environment variables.
type Cfg struct {
	Port     int
	DataDir  string
	APIKey   string
	AuthMode string // "none", "basic", "external"

	BasicAuthUsername string
	BasicAuthPassword string

	ExternalAuthURL string

	BehindTLS              bool
	OTLPCORSAllowedOrigins []string

	SSEQueueMax    int
	WriteQueueSize int

	ClickHouseDSN     string // Native protocol DSN for schema/ping.
	ClickHouseHTTPURL string // HTTP API URL for JSONEachRow inserts.
	Testing           bool
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Cfg {
	c := Cfg{
		Port:                   envInt("SOBS_PORT", 44317),
		DataDir:                envStr("SOBS_DATA_DIR", "data"),
		APIKey:                 envStr("SOBS_API_KEY", ""),
		AuthMode:               envStr("SOBS_AUTH_MODE", ""),
		BasicAuthUsername:      envStr("SOBS_AUTH_USERNAME", ""),
		BasicAuthPassword:      envStr("SOBS_AUTH_PASSWORD", ""),
		ExternalAuthURL:        envStr("SOBS_EXTERNAL_AUTH_URL", ""),
		BehindTLS:              envBool("SOBS_BEHIND_TLS", false),
		OTLPCORSAllowedOrigins: envCSV("SOBS_OTLP_CORS_ALLOWED_ORIGINS", "http://localhost:*,https://localhost:*,http://127.0.0.1:*,https://127.0.0.1:*"),
		SSEQueueMax:            envInt("SOBS_SSE_QUEUE_MAX", 200),
		WriteQueueSize:         envInt("SOBS_WRITE_QUEUE_SIZE", 1024),
		ClickHouseDSN:          envStr("SOBS_CLICKHOUSE_DSN", ""),
		ClickHouseHTTPURL:      envStr("SOBS_CLICKHOUSE_HTTP_URL", "http://localhost:8123"),
		Testing:                envStr("SOBS_TESTING", "") != "",
	}
	if c.AuthMode == "" {
		c.AuthMode = c.resolveAuthMode()
	}
	return c
}

func (c Cfg) resolveAuthMode() string {
	if c.ExternalAuthURL != "" {
		return "external"
	}
	if c.BasicAuthUsername != "" && c.BasicAuthPassword != "" {
		return "basic"
	}
	return "none"
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "TRUE", "True", "yes", "YES", "Yes", "on", "ON", "On":
		return true
	case "0", "false", "FALSE", "False", "no", "NO", "No", "off", "OFF", "Off":
		return false
	default:
		return fallback
	}
}

func envCSV(key, fallback string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		raw = fallback
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}
