package client_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/quay/terraform-provider-quay/internal/client"
)

func TestCreateOrganization_Conflict(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"message": "A user or organization with this name already exists",
		})
	})
	_, err := c.CreateOrganization(context.Background(), client.CreateOrganizationRequest{
		Name: "existing-org",
	})
	if err == nil {
		t.Fatal("expected error for conflict, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("conflict should not be IsNotFound")
	}
}

func TestCreateOrganization_Unauthorized(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"message":    "Unauthorized",
			"error_type": "invalid_token",
		})
	})
	_, err := c.CreateOrganization(context.Background(), client.CreateOrganizationRequest{
		Name: "myorg",
	})
	if err == nil {
		t.Fatal("expected error for unauthorized, got nil")
	}
}

func TestUpdateOrganization_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	_, err := c.UpdateOrganization(context.Background(), "missing-org", client.UpdateOrganizationRequest{
		Email: "new@example.com",
	})
	if err == nil {
		t.Fatal("expected error for update on non-existent org, got nil")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound for missing org update, got: %v", err)
	}
}

func TestDeleteOrganization_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	err := c.DeleteOrganization(context.Background(), "missing-org")
	if err == nil {
		t.Fatal("expected error for delete on non-existent org, got nil")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound for missing org delete, got: %v", err)
	}
}

func TestDeleteOrganization_Forbidden(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"message":    "You do not have permission to perform this action",
			"error_type": "insufficient_scope",
		})
	})
	err := c.DeleteOrganization(context.Background(), "other-org")
	if err == nil {
		t.Fatal("expected error for forbidden delete, got nil")
	}
	if client.IsNotFound(err) {
		t.Error("forbidden should not be IsNotFound")
	}
}
