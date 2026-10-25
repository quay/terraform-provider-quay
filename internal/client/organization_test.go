package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/quay/terraform-provider-quay/internal/client"
)

// boolPtr, intPtr and strPtr construct pointer-typed Organization fields in test literals.
func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }

// TestOrganization_MarshalZeroValues ensures zero/false values survive JSON
// round-tripping. omitempty silently drops them, breaking any code that
// re-serialises the struct or relies on the zero value being present.
func TestOrganization_MarshalZeroValues(t *testing.T) {
	org := client.Organization{
		Name:                "myorg",
		Email:               "",
		InvoiceEmail:        boolPtr(false),
		InvoiceEmailAddress: strPtr(""),
		TagExpirationS:      intPtr(0),
		IsFreeAccount:       boolPtr(false),
	}

	data, err := json.Marshal(org)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	requiredKeys := []string{
		"invoice_email",
		"invoice_email_address",
		"tag_expiration_s",
		"is_free_account",
	}
	for _, key := range requiredKeys {
		if _, exists := got[key]; !exists {
			t.Errorf("key %q was missing from marshalled Organization; omitempty is dropping the zero value", key)
		}
	}
}

func TestCreateOrganization(t *testing.T) {
	created := false
	srv, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organization/"):
			created = true
			writeJSON(w, http.StatusCreated, map[string]string{})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/organization/myorg"):
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", Email: "a@b.com"})
		default:
			http.NotFound(w, r)
		}
	})
	_ = srv

	org, err := c.CreateOrganization(context.Background(), client.CreateOrganizationRequest{
		Name:  "myorg",
		Email: "a@b.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("POST /organization/ was never called")
	}
	if org.Name != "myorg" {
		t.Errorf("expected name myorg, got %q", org.Name)
	}
}

func TestGetOrganization_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	_, err := c.GetOrganization(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestUpdateOrganization(t *testing.T) {
	updated := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			updated = true
			writeJSON(w, http.StatusOK, map[string]string{})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", Email: "new@b.com"})
		}
	})
	org, err := c.UpdateOrganization(context.Background(), "myorg", client.UpdateOrganizationRequest{
		Email: "new@b.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Error("PUT was never called")
	}
	if org.Email != "new@b.com" {
		t.Errorf("expected email new@b.com, got %q", org.Email)
	}
}

func TestDeleteOrganization(t *testing.T) {
	deleted := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		}
	})
	if err := c.DeleteOrganization(context.Background(), "myorg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("DELETE was never called")
	}
}

// TestUpdateOrganization_InvoiceEmailFalse ensures false is sent in the PUT body.
// omitempty on a bool field silently drops false, so Quay keeps the old value.
func TestUpdateOrganization_InvoiceEmailFalse(t *testing.T) {
	var gotBody map[string]any
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			writeJSON(w, http.StatusOK, map[string]string{})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", InvoiceEmail: boolPtr(false)})
		}
	})

	_, err := c.UpdateOrganization(context.Background(), "myorg", client.UpdateOrganizationRequest{
		InvoiceEmail: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, exists := gotBody["invoice_email"]
	if !exists {
		t.Fatal("invoice_email was missing from the PUT request body")
	}
	if v != false {
		t.Errorf("expected invoice_email=false in request body, got %v", v)
	}
}

// TestUpdateOrganization_EmailEmpty ensures an empty string is sent in the PUT body.
// Quay treats "" as a request to clear the field; omitempty drops it silently.
func TestUpdateOrganization_EmailEmpty(t *testing.T) {
	var gotBody map[string]any
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			writeJSON(w, http.StatusOK, map[string]string{})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", Email: ""})
		}
	})

	_, err := c.UpdateOrganization(context.Background(), "myorg", client.UpdateOrganizationRequest{
		Email: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, exists := gotBody["email"]
	if !exists {
		t.Fatal("email was missing from the PUT request body; omitempty prevents clearing the field")
	}
	if v != "" {
		t.Errorf("expected email=\"\" in request body, got %v", v)
	}
}

// TestUpdateOrganization_InvoiceEmailAddressEmpty ensures an empty string is sent in the PUT body.
// The Quay API schema marks this field nullable; omitempty drops "" silently.
func TestUpdateOrganization_InvoiceEmailAddressEmpty(t *testing.T) {
	var gotBody map[string]any
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			writeJSON(w, http.StatusOK, map[string]string{})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", InvoiceEmailAddress: strPtr("")})
		}
	})

	_, err := c.UpdateOrganization(context.Background(), "myorg", client.UpdateOrganizationRequest{
		InvoiceEmailAddress: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, exists := gotBody["invoice_email_address"]
	if !exists {
		t.Fatal("invoice_email_address was missing from the PUT request body; omitempty prevents clearing the field")
	}
	if v != "" {
		t.Errorf("expected invoice_email_address=\"\" in request body, got %v", v)
	}
}

// TestUpdateOrganization_TagExpirationZero ensures 0 is sent in the PUT body.
// omitempty on an int field silently drops 0, preventing users from clearing the expiration.
func TestUpdateOrganization_TagExpirationZero(t *testing.T) {
	var gotBody map[string]any
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			writeJSON(w, http.StatusOK, map[string]string{})
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Organization{Name: "myorg", TagExpirationS: intPtr(0)})
		}
	})

	_, err := c.UpdateOrganization(context.Background(), "myorg", client.UpdateOrganizationRequest{
		TagExpirationS: 0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, exists := gotBody["tag_expiration_s"]
	if !exists {
		t.Fatal("tag_expiration_s was missing from the PUT request body")
	}
	if v != float64(0) {
		t.Errorf("expected tag_expiration_s=0 in request body, got %v", v)
	}
}

// TestCreateOrganization_GetFailsAfterPost verifies the error message makes clear the org
// was already created. Without that, operators re-run apply, hit a 409 conflict, and are
// left with an orphaned resource and no Terraform state entry.
func TestCreateOrganization_GetFailsAfterPost(t *testing.T) {
	postCalled := false
	getCalled := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/organization/"):
			postCalled = true
			writeJSON(w, http.StatusCreated, map[string]string{})
		case r.Method == http.MethodGet:
			getCalled = true
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "unavailable"})
		default:
			http.NotFound(w, r)
		}
	})

	_, err := c.CreateOrganization(context.Background(), client.CreateOrganizationRequest{
		Name:  "myorg",
		Email: "a@b.com",
	})

	if !postCalled {
		t.Fatal("POST /organization/ was never called")
	}
	if !getCalled {
		t.Fatal("GET /organization/myorg was never called")
	}
	if err == nil {
		t.Fatal("expected error when GET fails after POST, got nil")
	}
	if !strings.Contains(err.Error(), "already created") {
		t.Errorf("error should mention that the org was already created; got: %v", err)
	}
}
