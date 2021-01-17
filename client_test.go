package client

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testServer(t *testing.T, resp string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte(resp))
	}))
}

func Test_signRequestBody(t *testing.T) {
	signature, err := signRequestBody("public key", "secret", []byte("{}"))

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
		BaseURL:    server.URL,
		logger:     log.New(ioutil.Discard, "", 0),
	}

	err := client.Call("any.method", &struct{}{}, &struct{}{})
	assert.NoError(t, err)
}
func TestClient_Call_Success(t *testing.T) {
	server := testServer(t, `{"jsonrpc": "2.0","result": {"key": "Value"},"id": "1"}`)

	client := apiClient{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		logger:     log.New(ioutil.Discard, "", 0),
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
	server := testServer(t, `{"jsonrpc": "2.0","error": {"code": 1, "message": "test error"},"id": "1"}`)

	client := apiClient{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		logger:     log.New(ioutil.Discard, "", 0),
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
		HTTPClient:     server.Client(),
		BaseURL:        server.URL,
		RequestRetrier: NewConstantRetry(uint(2), 0),
		logger:         log.New(ioutil.Discard, "", 0),
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
		HTTPClient:     server.Client(),
		BaseURL:        server.URL,
		RequestRetrier: NewConstantRetry(uint(1), 0),
		logger:         log.New(ioutil.Discard, "", 0),
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
		HTTPClient:     server.Client(),
		BaseURL:        server.URL,
		RequestRetrier: NewConstantRetry(uint(1), 0),
		logger:         log.New(ioutil.Discard, "", 0),
	}

	err := client.Call("any.method", &struct{}{}, &struct{}{})

	assert.Error(t, err)
	assert.Equal(t, "unexpected HTTP status: 401 Unauthorized", err.Error())
}
