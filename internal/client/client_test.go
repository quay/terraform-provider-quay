package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/quay/terraform-provider-quay/internal/client"
)

// newTestServer starts an httptest server and returns a Client pointed at it with zero retry delay.
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *client.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, client.NewTestClient(srv.URL, "test-token")
}

// writeJSON serialises v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ----- IsNotFound ---------------------------------------------------------

func TestIsNotFound_True(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	_, err := c.GetRepository(context.Background(), "org/missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound=true, got false for %v", err)
	}
}

func TestIsNotFound_False(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "boom"})
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if client.IsNotFound(err) {
		t.Errorf("expected IsNotFound=false, got true for %v", err)
	}
}

// ----- Authorization header -----------------------------------------------

// ----- Timeout ------------------------------------------------------------

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block long enough for a short timeout to fire.
		time.Sleep(500 * time.Millisecond)
		writeJSON(w, http.StatusOK, client.Repository{Namespace: "org", Name: "repo"})
	}))
	t.Cleanup(srv.Close)

	c := client.New(srv.URL, "test-token", 50*time.Millisecond)
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	// A timeout produces a transport-level error, not an APIError.
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		t.Errorf("expected a transport-level timeout error, got APIError: %v", apiErr)
	}
}

// ----- Retry behaviour ----------------------------------------------------

// TestRetry_GET5xx verifies that GET requests are retried on transient 5xx responses.
func TestRetry_GET5xx(t *testing.T) {
	attempts := 0
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, client.Repository{Namespace: "org", Name: "repo"})
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

// TestRetry_POST5xx verifies that POST requests are not retried on 5xx errors
// because a committed POST may have already mutated server state.
func TestRetry_POST5xx(t *testing.T) {
	attempts := 0
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "unavailable"})
	})
	_, err := c.CreateOrganization(context.Background(), client.CreateOrganizationRequest{Name: "org", Email: "a@b.com"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("POST should not be retried: expected 1 attempt, got %d", attempts)
	}
}

// TestRetry_429 verifies that 429 Too Many Requests is not retried.
func TestRetry_429(t *testing.T) {
	attempts := 0
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"message": "rate limited"})
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("429 should not be retried: expected 1 attempt, got %d", attempts)
	}
}

// ----- Redirect behaviour -------------------------------------------------

// TestRedirect_NonGETRefused verifies that redirects on non-GET requests are refused.
func TestRedirect_NonGETRefused(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{})
	}))
	t.Cleanup(target.Close)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusMovedPermanently)
	}))
	t.Cleanup(origin.Close)

	c := client.NewTestClient(origin.URL, "test-token")
	err := c.DeleteRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error for redirected DELETE, got nil")
	}
}

// TestRedirect_GETAllowed verifies that GET redirects are followed normally.
func TestRedirect_GETAllowed(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, client.Repository{Namespace: "org", Name: "repo"})
	}))
	t.Cleanup(target.Close)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusMovedPermanently)
	}))
	t.Cleanup(origin.Close)

	c := client.NewTestClient(origin.URL, "test-token")
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err != nil {
		t.Fatalf("expected GET redirect to succeed, got: %v", err)
	}
}

// TestRedirect_GETStripsAuthOnDowngrade verifies that Authorization is not forwarded
// on an https->http redirect. Go strips it on cross-host redirects but not on
// same-host downgrades, which would leak the bearer token in plaintext.
func TestRedirect_GETStripsAuthOnDowngrade(t *testing.T) {
	var receivedAuth string

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		writeJSON(w, http.StatusOK, client.Repository{Namespace: "org", Name: "repo"})
	}))
	t.Cleanup(target.Close)

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusMovedPermanently)
	}))
	t.Cleanup(origin.Close)

	c := client.NewTestTLSClient(origin.URL, "test-token", origin.Client())
	_, _ = c.GetRepository(context.Background(), "org/repo")

	if receivedAuth != "" {
		t.Errorf("Authorization header was forwarded on https->http redirect: %q — token leaked over plaintext", receivedAuth)
	}
}

// TestHTTPURLRejected verifies that http:// URLs are rejected to prevent the bearer token leaking in plaintext.
func TestHTTPURLRejected(t *testing.T) {
	err := client.ValidateURL("http://quay.example.com")
	if err == nil {
		t.Error("expected error for http:// URL, got nil — token would be sent in plaintext")
	}
}

func TestHTTPSURLAccepted(t *testing.T) {
	if err := client.ValidateURL("https://quay.example.com"); err != nil {
		t.Errorf("unexpected error for https:// URL: %v", err)
	}
}

// TestEmptyURLAccepted verifies that an empty URL defaults to https://quay.io.
func TestEmptyURLAccepted(t *testing.T) {
	if err := client.ValidateURL(""); err != nil {
		t.Errorf("unexpected error for empty URL: %v", err)
	}
}

// ----- Authorization header -----------------------------------------------

func TestAuthorizationHeader(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, client.Repository{Namespace: "org", Name: "repo"})
	})
	org, err := c.GetRepository(context.Background(), "org/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if org == nil {
		t.Fatal("expected non-nil organization")
	}
}
