package config

import (
	"os"
	"strconv"
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

	SSEQueueMax    int
	WriteQueueSize int

	ClickHouseDSN     string // Native protocol DSN for schema/ping.
	ClickHouseHTTPURL string // HTTP API URL for JSONEachRow inserts.
	Testing           bool
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Cfg {
	c := Cfg{
		Port:             envInt("SOBS_PORT", 44317),
		DataDir:          envStr("SOBS_DATA_DIR", "data"),
		APIKey:           envStr("SOBS_API_KEY", ""),
		AuthMode:         envStr("SOBS_AUTH_MODE", ""),
		BasicAuthUsername: envStr("SOBS_AUTH_USERNAME", ""),
		BasicAuthPassword: envStr("SOBS_AUTH_PASSWORD", ""),
		ExternalAuthURL:  envStr("SOBS_EXTERNAL_AUTH_URL", ""),
		SSEQueueMax:      envInt("SOBS_SSE_QUEUE_MAX", 200),
		WriteQueueSize:   envInt("SOBS_WRITE_QUEUE_SIZE", 1024),
		ClickHouseDSN:     envStr("SOBS_CLICKHOUSE_DSN", ""),
		ClickHouseHTTPURL: envStr("SOBS_CLICKHOUSE_HTTP_URL", "http://localhost:8123"),
		Testing:           envStr("SOBS_TESTING", "") != "",
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
