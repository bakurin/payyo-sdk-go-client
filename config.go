package client

import "time"

var (
	defaultRetryWaitMin = 1 * time.Second
	defaultRetryWaitMax = 30 * time.Second
	defaultRetryMax     = 4
)

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
		publicKey:    publicKey,
		secret:       secret,
		BaseURL:      BaseURLV3,
		Logger:       NewNullLogger(),
		RetryWaitMin: defaultRetryWaitMin,
		RetryWaitMax: defaultRetryWaitMax,
		RetryMax:     defaultRetryMax,
	}
}
