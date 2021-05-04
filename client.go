package client

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

const (
	// BaseURLV3 is base url for API version 3
	BaseURLV3 = "https://api.client.ch/v3"
)

var (
	defaultRequestBackoff = ExponentialJitterBackoff
	defaultRequestSigner  = Hmac256Signer
)

// Client is provided methods to all API
type Client interface {
	Call(method string, params, result interface{}) error
	CallWithContext(ctx context.Context, method string, params, result interface{}) error
}

type apiClient struct {
	Config         *Config
	HTTPClient     *http.Client
	RequestBackoff Backoff
	RequestSigner  Signer
}

// New creates a new client instance
func New(config *Config) Client {
	return &apiClient{
		Config: config,
		HTTPClient: &http.Client{
			Timeout: time.Second * 60,
		},
		RequestBackoff: defaultRequestBackoff,
		RequestSigner:  defaultRequestSigner,
	}
}

// Backoff allows to define different backoff scenarios to request retries
type Backoff func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration

func retryAfter(resp *http.Response) time.Duration {
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		if sleep, err := strconv.ParseInt(resp.Header.Get("Retry-After"), 10, 64); err == nil {
			return time.Second * time.Duration(sleep)
		}
	}

	return 0
}

// LinearJitterBackoff linearly increased the backoff with jitter
func LinearJitterBackoff(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
	delay := retryAfter(resp)
	if delay > 0 {
		return delay
	}

	rnd := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))
	jitter := rnd.Float64() * float64(max-min)
	jitterMin := int64(jitter) + int64(min)
	return time.Duration(jitterMin * int64(attemptNum))
}

// ExponentialJitterBackoff returns exponential backoff with jitter
// seep = rand(minDelay, min(maxDelay, base * 2 ** attemptNum))
func ExponentialJitterBackoff(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
	delay := retryAfter(resp)
	if delay > 0 {
		return delay
	}

	// nolint:gosec // math/rand is strong enough for this case
	rnd := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))

	base := float64(min) * float64(attemptNum)
	maxDelay := math.Min(float64(max), base*math.Pow(2.0, float64(attemptNum)))

	if float64(min) > maxDelay { // it's unclear what to do in such case
		maxDelay = float64(max)
	}

	jitter := rnd.Float64() * (maxDelay - float64(min))
	jitterMin := int64(jitter) + int64(min)

	return min + time.Duration(jitterMin)
}

// Signer is an interface of function to sign request body
type Signer func(publicKey, secret string, body []byte) (string, error)

// Hmac256Signer is default request signer
func Hmac256Signer(publicKey, secret string, body []byte) (string, error) {
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

// Call the RPC method
func (c apiClient) Call(method string, params, result interface{}) error {
	return c.CallWithContext(context.Background(), method, params, result)
}

// CallWithContext is the same as Call but allows to pass a context
func (c apiClient) CallWithContext(ctx context.Context, method string, params, result interface{}) error {
	rpcReq := newRPCRequest(method, params, "1")
	body, err := json.Marshal(rpcReq)

	c.log(DebugLevel, "request body: %s", body)

	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	signer := c.RequestSigner
	if signer == nil {
		signer = defaultRequestSigner
	}

	signature, err := signer(c.Config.publicKey, c.Config.secret, body)
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

func (c *apiClient) sendRequest(req *http.Request, v interface{}) error {
	var attempt int
	var resp *http.Response
	var doErr, checkErr error
	var shouldRetry bool

	retry := c.RequestBackoff
	if retry == nil {
		retry = defaultRequestBackoff
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
		shouldRetry, checkErr = checkRetry(req.Context(), resp, c.Config.RetryMax, attempt, doErr)

		if doErr != nil {
			c.log(ErrorLevel, "%s %s request failed: %v", req.Method, req.URL, doErr)
		}

		if !shouldRetry {
			break
		}

		// consume any response to reuse the connection.
		if doErr == nil {
			c.drainBody(resp.Body)
		}

		wait := retry(c.Config.RetryWaitMin, c.Config.RetryWaitMax, attempt, resp)
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

func checkRetry(ctx context.Context, resp *http.Response, retryMax, attemptNum int, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	shouldRetry, err := retryPolicy(resp, err)
	if attemptNum >= retryMax {
		return false, err
	}

	return shouldRetry, err
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

func (c apiClient) drainBody(body io.ReadCloser) {
	defer body.Close()
	_, err := io.Copy(ioutil.Discard, io.LimitReader(body, int64(4096)))
	if err != nil {
		c.log(ErrorLevel, "error reading response body: %v", err)
	}
}

func (c apiClient) log(level LogLevel, format string, args ...interface{}) {
	if c.Config.Logger != nil {
		c.Config.Logger.Logf(level, format, args...)
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
