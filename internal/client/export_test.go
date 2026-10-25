package client

import (
	"net/http"
	"time"
)

// NewTestClient creates a client with zero retry delays and no HTTP timeout for use in tests.
func NewTestClient(baseURL, token string) *Client {
	return New(baseURL, token, 0).withRetryDelay(func(int) time.Duration { return 0 })
}

// NewTestTLSClient creates a TLS-capable test client. The CheckRedirect policy
// from New is applied so redirect behaviour is consistent with production.
func NewTestTLSClient(baseURL, token string, httpClient *http.Client) *Client {
	c := New(baseURL, token, 0).withRetryDelay(func(int) time.Duration { return 0 })
	httpClient.CheckRedirect = c.httpClient.CheckRedirect
	c.httpClient = httpClient
	return c
}
