package config

type Config struct {
	HTTPAddr          string
	GRPCAddr          string
	TemplateRoot      string
	SessionCookieName string
	TrustedProxyMode  bool
	EnforceAPIAuth    bool
}

func Default() Config {
	return Config{
		HTTPAddr:          ":8080",
		GRPCAddr:          ":4317",
		TemplateRoot:      "templates",
		SessionCookieName: "session",
		TrustedProxyMode:  false,
		EnforceAPIAuth:    true,
	}
}
