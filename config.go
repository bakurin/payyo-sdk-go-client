package client

import "time"

// Config is client configuration object
type Config struct {
	publicKey    string
	secret       string
	BaseURL      string
	Logger       Logger
	RetryWaitMin time.Duration // Minimum time to wait
	RetryWaitMax time.Duration // Maximum time to wait
	RetryMax     int           // Maximum number of retries
}

// NewConfig initializes a client configuration
func NewConfig(publicKey, secret string) *Config {
	return &Config{
		publicKey: publicKey,
		secret:    secret,
		BaseURL:   BaseURLV3,
		Logger:    NewNullLogger(),
		RetryMax:  1,
	}
}
