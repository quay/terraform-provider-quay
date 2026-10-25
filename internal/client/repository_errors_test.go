package client_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/quay/terraform-provider-quay/internal/client"
)

func TestCreateRepository_Conflict(t *testing.T) {
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
		t.Fatal("expected error for conflict, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("conflict should not be IsNotFound")
	}
}

func TestCreateRepository_Unauthorized(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"message":    "Unauthorized",
			"error_type": "invalid_token",
		})
	})
	_, err := c.CreateRepository(context.Background(), client.CreateRepositoryRequest{
		Repository: "myrepo",
		Namespace:  "myorg",
		Visibility: "private",
	})
	if err == nil {
		t.Fatal("expected error for unauthorized, got nil")
	}
}

func TestUpdateRepository_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	_, err := c.UpdateRepository(context.Background(), "myorg/missing", client.UpdateRepositoryRequest{
		Description: "updated",
	})
	if err == nil {
		t.Fatal("expected error for update on non-existent repo, got nil")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound for missing repo update, got: %v", err)
	}
}

func TestDeleteRepository_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	err := c.DeleteRepository(context.Background(), "myorg/missing")
	if err == nil {
		t.Fatal("expected error for delete on non-existent repo, got nil")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound for missing repo delete, got: %v", err)
	}
}

func TestDeleteRepository_Forbidden(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"message":    "Forbidden",
			"error_type": "insufficient_scope",
		})
	})
	err := c.DeleteRepository(context.Background(), "otherorg/repo")
	if err == nil {
		t.Fatal("expected error for forbidden delete, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("forbidden should not be IsNotFound")
	}
}

func TestChangeRepositoryVisibility_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	err := c.ChangeRepositoryVisibility(context.Background(), "myorg/missing",
		client.ChangeVisibilityRequest{Visibility: "public"})
	if err == nil {
		t.Fatal("expected error for visibility change on non-existent repo, got nil")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got: %v", err)
	}
}

func TestChangeRepositoryVisibility_InvalidValue(t *testing.T) {
	// Invalid values are rejected before any HTTP request is made.
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP request should not have been made for an invalid visibility value")
	})
	err := c.ChangeRepositoryVisibility(context.Background(), "myorg/repo",
		client.ChangeVisibilityRequest{Visibility: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid visibility, got nil")
	}
	if !strings.Contains(err.Error(), "invalid visibility") {
		t.Errorf("expected 'invalid visibility' in error, got: %v", err)
	}
}
