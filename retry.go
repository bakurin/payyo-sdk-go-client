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

var (
	defaultRetryDelay       = time.Duration(2)
	defaultMaxRetryAttempts = 3
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

		return true, nil
	}

	if resp.StatusCode == 0 || (resp.StatusCode >= 500 && resp.StatusCode != 501) {
		return true, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}

	return false, nil
}

// ConstantRetry allow to retry within constant time intervals
type ConstantRetry struct {
	RetryDelay       time.Duration
	MaxRetryAttempts int
}

// NewLinearRetry creates a new instance of NewLinearRetry
func NewLinearRetry(delay time.Duration) *ConstantRetry {
	if delay == 0 {
		delay = defaultRetryDelay
	}

	return &ConstantRetry{
		RetryDelay:       delay,
		MaxRetryAttempts: defaultMaxRetryAttempts,
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

	if attemptNum >= r.MaxRetryAttempts {
		return false, fmt.Errorf("attempts limit (%d) axided", r.MaxRetryAttempts)
	}

	shouldRetry, _ := retryPolicy(resp, err)
	return shouldRetry, nil
}
