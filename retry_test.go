package client

import (
	"context"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
	"time"
)

func TestNoRetry_Backoff(t *testing.T) {
	retry := NewNoRetry()
	assert.Equal(t, time.Duration(0), retry.Backoff(1, &http.Response{}))
}

func TestNoRetry_CheckRetry_OnSuccess(t *testing.T) {
	retry := NewNoRetry()
	resp := &http.Response{
		StatusCode: 200,
	}

	shouldRetry, err := retry.CheckRetry(context.Background(), resp, 1, nil)

	assert.NoError(t, err)
	assert.False(t, shouldRetry)
}

func TestNoRetry_CheckRetry_OnFailure(t *testing.T) {
	retry := NewNoRetry()
	resp := &http.Response{
		Status:     http.StatusText(http.StatusUnauthorized),
		StatusCode: http.StatusUnauthorized,
	}

	shouldRetry, err := retry.CheckRetry(context.Background(), resp, 1, nil)

	assert.False(t, shouldRetry)
	assert.EqualError(t, err, "unexpected HTTP status: Unauthorized")
}
