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

// RequestRetryer defines an interface to support different retry policies
type RequestRetryer interface {
	CheckRetry(ctx context.Context, resp *http.Response, attemptNum int, err error) (bool, error)
	Backoff(attemptNum int, resp *http.Response) time.Duration
}

// NopRequestRetryer is a dummy implementation of RequestRetryer
type NopRequestRetryer struct {
}

// NewNopRequestRetryer creates a new NopRequestRetryer instance
func NewNopRequestRetryer() *NopRequestRetryer {
	return &NopRequestRetryer{}
}

// CheckRetry checks is new retry is needed
func (r NopRequestRetryer) CheckRetry(ctx context.Context, resp *http.Response, attemptNum int, err error) (bool, error) {
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}

	_, err = retryPolicy(resp, err)

	return false, err
}

// Backoff returns a delay before next attempt
func (r NopRequestRetryer) Backoff(attemptNum int, resp *http.Response) time.Duration {
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

// ConstantRequestRetryer allow to retry within constant time intervals
type ConstantRequestRetryer struct {
	RetryDelay       time.Duration
	MaxRetryAttempts uint
}

// NewConstantRequestRetryer creates a new instance of NewConstantRequestRetryer
func NewConstantRequestRetryer(maxRetries uint, delay time.Duration) *ConstantRequestRetryer {
	return &ConstantRequestRetryer{
		RetryDelay:       delay,
		MaxRetryAttempts: maxRetries,
	}
}

// Backoff returns a delay before upcoming attempt
func (r ConstantRequestRetryer) Backoff(attemptNum int, resp *http.Response) time.Duration {
	return r.RetryDelay
}

// CheckRetry checks if another attempt is needed
func (r ConstantRequestRetryer) CheckRetry(ctx context.Context, resp *http.Response, attemptNum int, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	shouldRetry, err := retryPolicy(resp, err)
	if uint(attemptNum) >= r.MaxRetryAttempts {
		return false, err
	}

	return shouldRetry, err
}
