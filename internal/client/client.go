package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://quay.io"

// DefaultTimeout is the HTTP client timeout used when none is specified.
const DefaultTimeout = 30 * time.Second

// ValidateURL returns an error if the URL is not acceptable. An empty string
// defaults to https://quay.io. http:// is rejected for non-loopback hosts
// because the bearer token would be sent in plaintext.
func ValidateURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return fmt.Errorf("http:// URLs are not permitted — the bearer token would be sent in plaintext; use https://")
	}
	return nil
}

// isLoopback reports whether host is a loopback address. http:// is permitted
// for loopback hosts to support local development and acceptance testing.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Client is a Quay API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	retryDelay func(attempt int) time.Duration
}

// New creates a new Quay API client. Pass 0 to disable the timeout.
func New(baseURL, token string, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
			// Go silently rewrites the method to GET on redirect (per RFC 7231);
			// non-GET redirects must be refused so a redirected DELETE is not
			// reported as success.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 0 && via[0].Method != http.MethodGet {
					return http.ErrUseLastResponse
				}
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				// Go does not strip Authorization on same-host https->http
				// redirects, so the bearer token would leak in plaintext.
				if req.URL.Scheme == "http" {
					req.Header.Del("Authorization")
				}
				return nil
			},
		},
		retryDelay: func(attempt int) time.Duration {
			return time.Duration(attempt) * time.Second
		},
	}
}

// withRetryDelay returns a shallow copy of the client with a custom retry delay function.
func (c *Client) withRetryDelay(f func(attempt int) time.Duration) *Client {
	copy := *c
	copy.retryDelay = f
	return &copy
}

// APIError represents an error response from the Quay API.
type APIError struct {
	StatusCode int
	Message    string
	ErrorType  string `json:"error_type"`
}

func (e *APIError) Error() string {
	if e.ErrorType != "" {
		return fmt.Sprintf("quay API error %d (%s): %s", e.StatusCode, e.ErrorType, e.Message)
	}
	return fmt.Sprintf("quay API error %d: %s", e.StatusCode, e.Message)
}

// IsNotFound returns true if the error chain contains a 404 Not Found APIError.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	return false
}

// IsForbidden returns true if the error chain contains a 403 Forbidden APIError.
func IsForbidden(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusForbidden
	}
	return false
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	// url.JoinPath percent-encodes '?', so the query string must be split off first.
	rawPath := path
	rawQuery := ""
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		rawPath = path[:idx]
		rawQuery = path[idx+1:]
	}

	u, err := url.JoinPath(c.baseURL, "/api/v1/", rawPath)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}
	if rawQuery != "" {
		u += "?" + rawQuery
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshalling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	return req, nil
}

// retryable5xxCodes are the transient server errors safe to retry on idempotent methods.
// POST is excluded because a committed POST may have already mutated state on the server.
var retryable5xxCodes = map[int]bool{
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

const maxRetries = 3

func (c *Client) do(req *http.Request, out any) error {
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("reading request body for retry: %w", err)
		}
		_ = req.Body.Close()
	}

	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			select {
			case <-req.Context().Done():
				return req.Context().Err()
			case <-time.After(c.retryDelay(attempt)):
			}
		}
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			req.ContentLength = int64(len(bodyBytes))
		}
		lastErr = c.doOnce(req, out)
		if lastErr == nil {
			return nil
		}
		var apiErr *APIError
		if !errors.As(lastErr, &apiErr) {
			return lastErr
		}
		if req.Method != http.MethodPost && retryable5xxCodes[apiErr.StatusCode] {
			continue
		}
		return lastErr
	}
	return lastErr
}

func (c *Client) doOnce(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 && req.Method != http.MethodGet {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("unexpected redirect for %s request", req.Method),
		}
	}

	if resp.StatusCode >= 400 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		var errBody struct {
			Detail       string `json:"detail"`
			ErrorMessage string `json:"error_message"`
			Message      string `json:"message"`
			ErrorType    string `json:"error_type"`
		}
		if json.Unmarshal(respBody, &errBody) == nil {
			apiErr.ErrorType = errBody.ErrorType
			switch {
			case errBody.Detail != "":
				apiErr.Message = errBody.Detail
			case errBody.ErrorMessage != "":
				apiErr.Message = errBody.ErrorMessage
			default:
				apiErr.Message = errBody.Message
			}
		}
		if apiErr.Message == "" {
			apiErr.Message = string(respBody)
		}
		return apiErr
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	req, err := c.newRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) put(ctx context.Context, path string, body, out any) error {
	req, err := c.newRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) delete(ctx context.Context, path string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}
