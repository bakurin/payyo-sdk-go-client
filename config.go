package client

// Config is client configuration object
type Config struct {
	publicKey string
	secret    string
	BaseURL   string
	Logger    Logger
}

// NewConfig initializes a client configuration
func NewConfig(publicKey, secret string) *Config {
	return &Config{
		publicKey: publicKey,
		secret:    secret,
		BaseURL:   BaseURLV3,
		Logger:    NewNullLogger(),
	}
}
