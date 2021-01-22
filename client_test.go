package client

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func testServer(resp string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte(resp))
	}))
}

func TestHmac256Signer(t *testing.T) {
	signature, err := Hmac256Signer("public key", "secret", []byte("{}"))

	assert.NoError(t, err)
	assert.Equal(t, "cHVibGljIGtleToyYTcyOTc1ZTIxZDgzZmRjZGY3Y2U1ZDY2ZGMzOTBlM2MzZWEwMGI3MjJlOTAzNmI5YTlhNjFkZDljMjIyNzk4", signature)
}

func TestClient_Call_RequestHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "application/json; charset=utf-8", req.Header.Get("Accept"))
		assert.Equal(t, "application/json; charset=utf-8", req.Header.Get("Content-Type"))
		assert.Equal(t, "Basic", req.Header.Get("Authorization")[0:5])

		_, _ = rw.Write([]byte("{}"))
	}))

	client := apiClient{
		HTTPClient: server.Client(),
		Config: &Config{
			BaseURL: server.URL,
		},
	}

	err := client.Call("any.method", &struct{}{}, &struct{}{})
	assert.NoError(t, err)
}
func TestClient_Call_Success(t *testing.T) {
	server := testServer(`{"jsonrpc": "2.0","result": {"key": "Value"},"id": "1"}`)

	client := apiClient{
		HTTPClient: server.Client(),
		Config: &Config{
			BaseURL: server.URL,
		},
	}

	params := struct{}{}
	result := &struct {
		Key string `json:"key"`
	}{}
	err := client.Call("any.method", params, result)

	assert.NoError(t, err)
	assert.Equal(t, "Value", result.Key)
}

func TestClient_Call_Error(t *testing.T) {
	server := testServer(`{"jsonrpc": "2.0","error": {"code": 1, "message": "test error"},"id": "1"}`)

	client := apiClient{
		HTTPClient: server.Client(),
		Config: &Config{
			BaseURL: server.URL,
		},
	}

	params := struct{}{}
	result := struct{}{}
	err := client.Call("any.method", params, result)

	assert.Error(t, err)
	assert.Equal(t, "test error (1)", fmt.Sprintf("%s", err))
}

func TestClient_Call_SuccessAfterRetry(t *testing.T) {
	var reqCounter int
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		reqCounter++
		if reqCounter <= 1 {
			// fail request on the first attempt
			rw.WriteHeader(500)
			return
		}

		_, _ = rw.Write([]byte(`{"jsonrpc": "2.0","result": {"key": "Value"},"id": "1"}`))
	}))

	client := apiClient{
		HTTPClient: server.Client(),
		Config: &Config{
			BaseURL:  server.URL,
			RetryMax: 2,
		},
	}

	result := &struct {
		Key string `json:"key"`
	}{}
	err := client.Call("any.method", struct{}{}, result)

	assert.NoError(t, err)
	assert.Equal(t, "Value", result.Key)
}

func TestClient_Call_FailAfterAllRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(500)
	}))

	client := apiClient{
		HTTPClient: server.Client(),
		Config: &Config{
			BaseURL:  server.URL,
			RetryMax: 2,
		},
	}

	err := client.Call("any.method", &struct{}{}, &struct{}{})

	assert.Error(t, err)
	assert.Equal(t, "unexpected HTTP status: 500 Internal Server Error", err.Error())
}

func TestClient_Call_DoNotRetry(t *testing.T) {
	var reqCounter int
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		reqCounter++
		assert.LessOrEqual(t, 1, reqCounter)
		rw.WriteHeader(401)
	}))

	client := apiClient{
		HTTPClient: server.Client(),
		Config: &Config{
			BaseURL:  server.URL,
			RetryMax: 0,
		},
	}

	err := client.Call("any.method", &struct{}{}, &struct{}{})

	assert.Error(t, err)
	assert.Equal(t, "unexpected HTTP status: 401 Unauthorized", err.Error())
}

func TestClient_retryPolicy_Status500(t *testing.T) {
	resp := &http.Response{
		Status:     http.StatusText(http.StatusInternalServerError),
		StatusCode: http.StatusInternalServerError,
	}

	shouldRetry, err := retryPolicy(resp, nil)
	assert.EqualError(t, err, "unexpected HTTP status: Internal Server Error")
	assert.True(t, shouldRetry)
}

func TestClient_retryPolicy_Status400(t *testing.T) {
	resp := &http.Response{
		Status:     http.StatusText(http.StatusBadRequest),
		StatusCode: http.StatusBadRequest,
	}

	shouldRetry, err := retryPolicy(resp, nil)
	assert.EqualError(t, err, "unexpected HTTP status: Bad Request")
	assert.False(t, shouldRetry)
}

func TestClient_checkRetry_TooManyRedirects(t *testing.T) {
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

func TestClient_retryPolicy_UnsupportedProtocolScheme(t *testing.T) {
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

func TestClient_retryPolicy_UnknownAuthority(t *testing.T) {
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

func TestLinearJitterBackoff(t *testing.T) {
	min := time.Second
	max := 2 * time.Second
	backoff := LinearJitterBackoff(min, max, 1, &http.Response{})

	assert.Greater(t, backoff.Nanoseconds(), min.Nanoseconds())
	assert.Less(t, backoff.Nanoseconds(), max.Nanoseconds())
}

func TestExponentialJitterBackoff(t *testing.T) {
	min := time.Second
	max := 60 * time.Second

	backoff1 := ExponentialJitterBackoff(min, max, 1, &http.Response{})
	backoff2 := ExponentialJitterBackoff(min, max, 2, &http.Response{})

	assert.Greater(t, backoff1.Nanoseconds(), min.Nanoseconds())
	assert.Less(t, backoff1.Nanoseconds(), max.Nanoseconds())

	assert.Greater(t, backoff2.Nanoseconds(), min.Nanoseconds())
	assert.Less(t, backoff2.Nanoseconds(), max.Nanoseconds())
}
