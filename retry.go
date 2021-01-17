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

type Retry interface {
	CheckRetry(ctx context.Context, resp *http.Response, attemptNum int, err error) (bool, error)
	Backoff(attemptNum int, resp *http.Response) time.Duration
}

type NoRetry struct {
}

func NewNoRetry() *NoRetry {
	return &NoRetry{}
}

func (r NoRetry) CheckRetry(ctx context.Context, resp *http.Response, attemptNum int, err error) (bool, error) {
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	return false, err
}

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

type LinearRetry struct {
	RetryDelay       time.Duration
	MaxRetryAttempts int
}

func NewLinearRetry(delay time.Duration) *LinearRetry {
	if delay == 0 {
		delay = defaultRetryDelay
	}

	return &LinearRetry{
		RetryDelay:       delay,
		MaxRetryAttempts: defaultMaxRetryAttempts,
	}
}

func (r LinearRetry) Backoff(attemptNum int, resp *http.Response) time.Duration {
	return r.RetryDelay
}

func (r LinearRetry) CheckRetry(ctx context.Context, resp *http.Response, attemptNum int, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if attemptNum >= r.MaxRetryAttempts {
		return false, fmt.Errorf("attempts limit (%d) axided", r.MaxRetryAttempts)
	}

	shouldRetry, _ := retryPolicy(resp, err)
	return shouldRetry, nil
}
