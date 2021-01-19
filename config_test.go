package client

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig("key", "secret")

	assert.Equal(t, "key", cfg.publicKey)
	assert.Equal(t, "secret", cfg.secret)
	assert.Equal(t, "https://api.client.ch/v3", cfg.BaseURL)
}
