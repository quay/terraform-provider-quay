package client_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/quay/terraform-provider-quay/internal/client"
)

// ----- HTTP error status codes --------------------------------------------

func TestAPIError_401_Unauthorized(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"message":    "Unauthorized",
			"error_type": "invalid_token",
		})
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("401 should not be IsNotFound")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401, got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid_token") {
		t.Errorf("error should mention error_type, got: %v", err)
	}
}

func TestAPIError_403_Forbidden(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"message":    "Forbidden",
			"error_type": "insufficient_scope",
		})
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("403 should not be IsNotFound")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention status 403, got: %v", err)
	}
}

func TestAPIError_409_Conflict(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"message": "Repository already exists",
		})
	})
	_, err := c.CreateRepository(context.Background(), client.CreateRepositoryRequest{
		Repository: "existing",
		Namespace:  "myorg",
		Visibility: "private",
	})
	if err == nil {
		t.Fatal("expected error for 409, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("409 should not be IsNotFound")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Errorf("error should mention status 409, got: %v", err)
	}
}

func TestAPIError_500_InternalServerError(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"message": "Internal server error",
		})
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("500 should not be IsNotFound")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500, got: %v", err)
	}
}

// TestAPIError_UsesDetailField verifies that the detail field is used in preference
// to the raw JSON body. Quay's primary human-readable error fields are detail and
// error_message; reading only message leaves users with raw JSON in error output.
func TestAPIError_UsesDetailField(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"detail":        "Requires authentication",
			"error_message": "Requires authentication",
			"error_type":    "invalid_token",
			"title":         "invalid_token",
			"status":        "401",
		})
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), `"detail"`) {
		t.Errorf("error message contains raw JSON key %q — detail field was not extracted: %v", "detail", err)
	}
	if !strings.Contains(err.Error(), "Requires authentication") {
		t.Errorf("error message should contain the detail value, got: %v", err)
	}
}

// TestAPIError_UsesErrorMessageField verifies that error_message is preferred when detail is absent.
func TestAPIError_UsesErrorMessageField(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error_message": "Invalid tag expiration",
			"error_type":    "invalid_request",
		})
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), `"error_message"`) {
		t.Errorf("error message contains raw JSON key — error_message field was not extracted: %v", err)
	}
	if !strings.Contains(err.Error(), "Invalid tag expiration") {
		t.Errorf("error message should contain the error_message value, got: %v", err)
	}
}

// ----- Invalid / non-JSON response body -----------------------------------

func TestAPIError_NonJSONErrorBody(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("plain text error from proxy"))
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error for non-JSON 500, got nil")
	}
	// Should still surface the raw body in the error message.
	if !strings.Contains(err.Error(), "plain text error from proxy") {
		t.Errorf("expected raw body in error, got: %v", err)
	}
}

func TestSuccessResponse_MalformedJSON(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name": "org", INVALID`))
	})
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decoding") {
		t.Errorf("expected decoding error, got: %v", err)
	}
}

// ----- Context cancellation -----------------------------------------------

func TestContextCancellation(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow response — context should cancel before this completes.
		<-r.Context().Done()
		writeJSON(w, http.StatusOK, client.Repository{Namespace: "org", Name: "repo"})
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := c.GetRepository(ctx, "org/repo")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ----- Empty token --------------------------------------------------------

// TestEmptyToken_StillSendsRequest verifies that token validation is the provider's
// responsibility, not the client's, and that the server's 401 still surfaces as an error.
func TestEmptyToken_StillSendsRequest(t *testing.T) {
	var requestReceived atomic.Bool
	srv, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestReceived.Store(true)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "no token"})
	})
	c := client.New(srv.URL, "", 0)
	_, err := c.GetRepository(context.Background(), "org/repo")
	if err == nil {
		t.Fatal("expected 401 error for empty token, got nil")
	}
	if !requestReceived.Load() {
		t.Error("expected the server to receive the request")
	}
}
