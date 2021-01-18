package client

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

// Retry defines an interface to support different retry policies
type Retry interface {
	CheckRetry(ctx context.Context, resp *http.Response, attemptNum int, err error) (bool, error)
	Backoff(attemptNum int, resp *http.Response) time.Duration
}

// NoRetry is a dummy implementation of Retry
type NoRetry struct {
}

// NewNoRetry creates a new NoRetry instance
func NewNoRetry() *NoRetry {
	return &NoRetry{}
}

// CheckRetry checks is new retry is needed
func (r NoRetry) CheckRetry(ctx context.Context, resp *http.Response, attemptNum int, err error) (bool, error) {
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}

	_, err = retryPolicy(resp, err)

	return false, err
}

// Backoff returns a delay before next attempt
func (r NoRetry) Backoff(attemptNum int, resp *http.Response) time.Duration {
	return 0
}

func retryPolicy(resp *http.Response, err error) (bool, error) {
	if err != nil {
		if v, ok := err.(*url.Error); ok {
			if regexp.MustCompile(`stopped after \d+ redirects\z`).MatchString(v.Error()) {
				return false, v
			}

			if regexp.MustCompile(`unsupported protocol scheme`).MatchString(v.Error()) {
				return false, v
			}

			// Don't retry if the error was due to TLS cert verification failure.
			if _, ok := v.Err.(x509.UnknownAuthorityError); ok {
				return false, v
			}
		}

		return true, err
	}

	// consider error codes of range 500 as recoverable
	if resp.StatusCode == 0 || (resp.StatusCode >= 500 && resp.StatusCode != 501) {
		return true, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	return false, nil
}

// ConstantRetry allow to retry within constant time intervals
type ConstantRetry struct {
	RetryDelay       time.Duration
	MaxRetryAttempts uint
}

// NewConstantRetry creates a new instance of NewConstantRetry
func NewConstantRetry(maxRetries uint, delay time.Duration) *ConstantRetry {
	return &ConstantRetry{
		RetryDelay:       delay,
		MaxRetryAttempts: maxRetries,
	}
}

// Backoff returns a delay before upcoming attempt
func (r ConstantRetry) Backoff(attemptNum int, resp *http.Response) time.Duration {
	return r.RetryDelay
}

// CheckRetry checks if another attempt is needed
func (r ConstantRetry) CheckRetry(ctx context.Context, resp *http.Response, attemptNum int, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	shouldRetry, err := retryPolicy(resp, err)
	if uint(attemptNum) >= r.MaxRetryAttempts {
		return false, err
	}

	return shouldRetry, err
}
