package client

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	// BaseURLV3 is base url for API version 3
	BaseURLV3 = "https://api.client.ch/v3"
)

var (
	defaultRequestRetryer = NewNopRequestRetryer()
)

// Client is provided methods to all API
type Client interface {
	Call(method string, params, result interface{}) error
	CallWithContext(ctx context.Context, method string, params, result interface{}) error
}

type apiClient struct {
	Config         *Config
	HTTPClient     *http.Client
	RequestRetryer RequestRetryer
}

// New creates a new client instance
func New(config *Config) Client {
	return &apiClient{
		Config: config,
		HTTPClient: &http.Client{
			Timeout: time.Second * 60,
		},
		RequestRetryer: defaultRequestRetryer,
	}
}

// Call the RPC method
func (c apiClient) Call(method string, params, result interface{}) error {
	return c.CallWithContext(context.Background(), method, params, result)
}

// CallWithContext is the same as Call but allows to pass a context
func (c apiClient) CallWithContext(ctx context.Context, method string, params, result interface{}) error {
	rpcReq := newRPCRequest(method, params, "1")
	body, err := json.Marshal(rpcReq)

	if c.Config.Logger != nil {
		c.Config.Logger.Logf("[DBG] request body: %s", body)
	}

	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	signature, err := signRequestBody(c.Config.publicKey, c.Config.secret, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Basic "+signature)

	err = c.sendRequest(req, result)
	if err != nil {
		return err
	}

	return nil
}

func signRequestBody(publicKey, secret string, body []byte) (string, error) {
	base64body := base64.RawURLEncoding.EncodeToString(body)
	hash := hmac.New(sha256.New, []byte(secret))
	_, err := hash.Write([]byte(base64body))
	if err != nil {
		return "", err
	}

	bodyHash := hex.EncodeToString(hash.Sum(nil))
	signature := fmt.Sprintf("%s:%s", publicKey, bodyHash)

	return base64.StdEncoding.EncodeToString([]byte(signature)), nil
}

func (c *apiClient) sendRequest(req *http.Request, v interface{}) error {
	var attempt int
	var resp *http.Response
	var doErr, checkErr error
	var shouldRetry bool

	retry := c.RequestRetryer
	if retry == nil {
		retry = defaultRequestRetryer
	}

	for {
		attempt++

		if req.Body != nil {
			body := req.Body
			if c, ok := body.(io.ReadCloser); ok {
				req.Body = c
			} else {
				req.Body = ioutil.NopCloser(body)
			}
		}

		resp, doErr = c.HTTPClient.Do(req)
		shouldRetry, checkErr = retry.CheckRetry(req.Context(), resp, attempt, doErr)

		if doErr != nil {
			c.log("[ERR] %s %s request failed: %v", req.Method, req.URL, doErr)
		}

		if !shouldRetry {
			break
		}

		// consume any response to reuse the connection.
		if doErr == nil {
			c.drainBody(resp.Body)
		}

		wait := retry.Backoff(attempt, resp)
		select {
		case <-req.Context().Done():
			c.HTTPClient.CloseIdleConnections()
			return req.Context().Err()
		case <-time.After(wait):
		}

		httpreq := *req
		req = &httpreq
	}

	if doErr == nil && checkErr == nil && !shouldRetry {
		rpcResponse := &rpcResponse{
			Result: v,
			Error:  nil,
		}
		err := json.NewDecoder(resp.Body).Decode(rpcResponse)
		if err != nil {
			return err
		}

		if rpcResponse.Error != nil {
			return fmt.Errorf("%s (%d)", rpcResponse.Error.Message, rpcResponse.Error.Code)
		}

		return nil
	}

	defer c.HTTPClient.CloseIdleConnections()

	err := doErr
	if checkErr != nil {
		err = checkErr
	}

	if resp != nil {
		c.drainBody(resp.Body)
	}

	if err == nil {
		return fmt.Errorf("%s %s giving up after %d attempt(s)", req.Method, req.URL, attempt)
	}

	return err
}

func (c apiClient) drainBody(body io.ReadCloser) {
	defer body.Close()
	_, err := io.Copy(ioutil.Discard, io.LimitReader(body, int64(4096)))
	if err != nil {
		c.log("[ERR] error reading response body: %v", err)
	}
}

func (c apiClient) log(format string, args ...interface{}) {
	if c.Config.Logger != nil {
		c.Config.Logger.Logf(format, args...)
	}
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      string      `json:"id"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
	ID      string      `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newRPCRequest(method string, params interface{}, id string) *rpcRequest {
	if id == "" {
		id = "1"
	}
	return &rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}
}
