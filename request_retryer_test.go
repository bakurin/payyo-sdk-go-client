package client

import (
	"context"
	"crypto/x509"
	"errors"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestNoRetry_Backoff(t *testing.T) {
	retry := NewNopRequestRetryer()
	assert.Equal(t, time.Duration(0), retry.Backoff(1, &http.Response{}))
}

func TestNoRetry_CheckRetry_OnSuccess(t *testing.T) {
	retry := NewNopRequestRetryer()
	resp := &http.Response{
		StatusCode: 200,
	}

	shouldRetry, err := retry.CheckRetry(context.Background(), resp, 1, nil)

	assert.NoError(t, err)
	assert.False(t, shouldRetry)
}

func TestNoRetry_CheckRetry_OnFailure(t *testing.T) {
	retry := NewNopRequestRetryer()
	resp := &http.Response{
		Status:     http.StatusText(http.StatusUnauthorized),
		StatusCode: http.StatusUnauthorized,
	}

	shouldRetry, err := retry.CheckRetry(context.Background(), resp, 1, nil)

	assert.False(t, shouldRetry)
	assert.EqualError(t, err, "unexpected HTTP status: Unauthorized")
}

func TestConstantRetry_Backoff(t *testing.T) {
	constDuration := time.Duration(42)
	retry := NewConstantRequestRetryer(1, constDuration)
	assert.Equal(t, constDuration, retry.Backoff(1, &http.Response{}))
	assert.Equal(t, constDuration, retry.Backoff(2, &http.Response{}))
}

func TestConstantRetry_CheckRetry_OnSuccess(t *testing.T) {
	retry := NewConstantRequestRetryer(1, time.Duration(1))
	resp := &http.Response{
		StatusCode: http.StatusOK,
	}

	shouldRetry, err := retry.CheckRetry(context.Background(), resp, 1, nil)
	assert.NoError(t, err)
	assert.False(t, shouldRetry)
}

func TestConstantRetry_CheckRetry_OnFailure(t *testing.T) {
	retry := NewConstantRequestRetryer(2, time.Duration(1))
	resp := &http.Response{
		Status:     http.StatusText(http.StatusUnauthorized),
		StatusCode: http.StatusUnauthorized,
	}

	shouldRetry, err := retry.CheckRetry(context.Background(), resp, 1, nil)
	assert.Error(t, err)
	assert.False(t, shouldRetry)
}

func TestConstantRetry_CheckRetry_OnRecoverableFailure(t *testing.T) {
	retry := NewConstantRequestRetryer(2, time.Duration(1))
	resp := &http.Response{
		Status:     http.StatusText(http.StatusInternalServerError),
		StatusCode: http.StatusInternalServerError,
	}

	shouldRetry, err := retry.CheckRetry(context.Background(), resp, 1, nil)
	assert.Error(t, err)
	assert.True(t, shouldRetry)

	shouldRetry, err = retry.CheckRetry(context.Background(), resp, 2, nil)
	assert.Error(t, err)
	assert.False(t, shouldRetry)
}

func TestConstantRetry_CheckRetry_OnContextCancelled(t *testing.T) {
	retry := NewConstantRequestRetryer(2, time.Duration(1))
	resp := &http.Response{
		Status:     http.StatusText(http.StatusUnauthorized),
		StatusCode: http.StatusUnauthorized,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	shouldRetry, err := retry.CheckRetry(ctx, resp, 1, nil)
	assert.EqualError(t, err, "context canceled")
	assert.False(t, shouldRetry)
}

func Test_retryPolicy_Status200(t *testing.T) {
	resp := &http.Response{
		Status:     http.StatusText(http.StatusOK),
		StatusCode: http.StatusOK,
	}

	respErr := &url.Error{
		Op:  "GET",
		Err: errors.New("http: nil Request.URL"),
	}
	shouldRetry, err := retryPolicy(resp, respErr)
	assert.EqualError(t, err, respErr.Error())
	assert.True(t, shouldRetry)
}

func Test_retryPolicy_Status500(t *testing.T) {
	resp := &http.Response{
		Status:     http.StatusText(http.StatusInternalServerError),
		StatusCode: http.StatusInternalServerError,
	}

	shouldRetry, err := retryPolicy(resp, nil)
	assert.EqualError(t, err, "unexpected HTTP status: Internal Server Error")
	assert.True(t, shouldRetry)
}

func Test_retryPolicy_Status400(t *testing.T) {
	resp := &http.Response{
		Status:     http.StatusText(http.StatusBadRequest),
		StatusCode: http.StatusBadRequest,
	}

	shouldRetry, err := retryPolicy(resp, nil)
	assert.EqualError(t, err, "unexpected HTTP status: Bad Request")
	assert.False(t, shouldRetry)
}

func Test_retryPolicy_TooManyRedirects(t *testing.T) {
	resp := &http.Response{
		Status:     http.StatusText(http.StatusBadRequest),
		StatusCode: http.StatusBadRequest,
	}

	respErr := &url.Error{
		Op:  "GET",
		URL: "https://example.net",
		Err: errors.New("stopped after 42 redirects"),
	}
	shouldRetry, err := retryPolicy(resp, respErr)
	assert.EqualError(t, err, `GET "https://example.net": stopped after 42 redirects`)
	assert.False(t, shouldRetry)
}

func Test_retryPolicy_UnsupportedProtocolScheme(t *testing.T) {
	resp := &http.Response{
		Status:     http.StatusText(http.StatusBadRequest),
		StatusCode: http.StatusBadRequest,
	}

	respErr := &url.Error{
		Op:  "GET",
		URL: "https://example.net",
		Err: errors.New("unsupported protocol scheme"),
	}
	shouldRetry, err := retryPolicy(resp, respErr)
	assert.EqualError(t, err, `GET "https://example.net": unsupported protocol scheme`)
	assert.False(t, shouldRetry)
}

func Test_retryPolicy_UnknownAuthority(t *testing.T) {
	resp := &http.Response{
		Status:     http.StatusText(http.StatusBadRequest),
		StatusCode: http.StatusBadRequest,
	}

	respErr := &url.Error{
		Op:  "GET",
		URL: "https://example.net",
		Err: x509.UnknownAuthorityError{},
	}
	shouldRetry, err := retryPolicy(resp, respErr)
	assert.EqualError(t, err, `GET "https://example.net": x509: certificate signed by unknown authority`)
	assert.False(t, shouldRetry)
}
