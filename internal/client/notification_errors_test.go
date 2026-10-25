package client_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/quay/terraform-provider-quay/internal/client"
)

func TestCreateRepoNotification_Unauthorized(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"message":    "Unauthorized",
			"error_type": "invalid_token",
		})
	})
	_, err := c.CreateRepoNotification(context.Background(), "myorg/myrepo",
		client.CreateNotificationRequest{
			Event:       "repo_push",
			Method:      "webhook",
			Config:      map[string]any{"url": "https://example.com"},
			EventConfig: map[string]any{},
		})
	if err == nil {
		t.Fatal("expected error for unauthorized, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("unauthorized should not be IsNotFound")
	}
}

func TestCreateRepoNotification_RepositoryNotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	_, err := c.CreateRepoNotification(context.Background(), "myorg/missing",
		client.CreateNotificationRequest{
			Event:       "repo_push",
			Method:      "webhook",
			Config:      map[string]any{"url": "https://example.com"},
			EventConfig: map[string]any{},
		})
	if err == nil {
		t.Fatal("expected error for missing repo, got nil")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound for missing repo, got: %v", err)
	}
}

// TestDeleteRepoNotification_NotFound verifies that a 404 on DELETE is treated as success
// because the desired end state — the notification does not exist — has been reached.
func TestDeleteRepoNotification_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	if err := c.DeleteRepoNotification(context.Background(), "myorg/myrepo", "missing-uuid"); err != nil {
		t.Errorf("expected nil for 404 on delete, got: %v", err)
	}
}

// TestDeleteRepoNotification_400NotificationNotFound verifies that Quay's 400
// "No repository notification found" is treated as success. Quay returns 400,
// not 404, for a DELETE on a notification that no longer exists.
func TestDeleteRepoNotification_400NotificationNotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"message": "No repository notification found",
		})
	})
	err := c.DeleteRepoNotification(context.Background(), "myorg/myrepo", "missing-uuid")
	if err != nil {
		t.Errorf("expected nil for 400 'No repository notification found', got: %v", err)
	}
}

func TestDeleteRepoNotification_Forbidden(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"message":    "Forbidden",
			"error_type": "insufficient_scope",
		})
	})
	err := c.DeleteRepoNotification(context.Background(), "otherorg/repo", "some-uuid")
	if err == nil {
		t.Fatal("expected error for forbidden delete, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("forbidden should not be IsNotFound")
	}
}
