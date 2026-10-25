package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/quay/terraform-provider-quay/internal/client"
)

func TestCreateRepoNotification(t *testing.T) {
	created := false
	getCalled := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		notif := client.Notification{
			UUID:        "abc-123",
			Event:       "repo_push",
			Method:      "webhook",
			Config:      map[string]any{"url": "https://example.com"},
			EventConfig: map[string]any{},
		}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notification/"):
			created = true
			writeJSON(w, http.StatusCreated, notif)
		case r.Method == http.MethodGet:
			getCalled = true
			writeJSON(w, http.StatusOK, notif)
		}
	})

	notif, err := c.CreateRepoNotification(context.Background(), "myorg/myrepo", client.CreateNotificationRequest{
		Event:       "repo_push",
		Method:      "webhook",
		Config:      map[string]any{"url": "https://example.com"},
		EventConfig: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("POST notification endpoint was never called")
	}
	// A GET after POST opens an orphan window: if the GET fails, the notification
	// fires indefinitely with no Terraform state entry tracking it.
	if getCalled {
		t.Error("CreateRepoNotification must not GET after POST; the POST response already contains the full notification")
	}
	if notif.UUID != "abc-123" {
		t.Errorf("expected UUID abc-123, got %q", notif.UUID)
	}
}

// TestCreateRepoNotification_GetFailsAfterPost demonstrates the orphan window:
// if a GET after a successful POST fails, the notification exists in Quay and
// keeps firing but Terraform has no state entry for it.
func TestCreateRepoNotification_GetFailsAfterPost(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notification/"):
			writeJSON(w, http.StatusCreated, client.Notification{
				UUID:        "abc-123",
				Event:       "repo_push",
				Method:      "webhook",
				Config:      map[string]any{"url": "https://example.com"},
				EventConfig: map[string]any{},
			})
		case r.Method == http.MethodGet:
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "unavailable"})
		}
	})

	notif, err := c.CreateRepoNotification(context.Background(), "myorg/myrepo", client.CreateNotificationRequest{
		Event:  "repo_push",
		Method: "webhook",
		Config: map[string]any{"url": "https://example.com"},
	})
	if err != nil {
		t.Errorf("CreateRepoNotification returned an error when the POST succeeded; "+
			"the notification exists in Quay but has no state entry: %v", err)
	}
	if notif == nil || notif.UUID != "abc-123" {
		t.Errorf("expected uuid abc-123 from POST response, got %v", notif)
	}
}

func TestGetRepoNotification(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/notification/") {
			writeJSON(w, http.StatusOK, map[string]any{
				"notifications": []client.Notification{
					{
						UUID:   "abc-123",
						Event:  "repo_push",
						Method: "webhook",
						Config: map[string]any{"url": "https://example.com"},
					},
				},
			})
		}
	})

	notif, err := c.GetRepoNotification(context.Background(), "myorg/myrepo", "abc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notif.UUID != "abc-123" {
		t.Errorf("expected UUID abc-123, got %q", notif.UUID)
	}
}

func TestDeleteRepoNotification(t *testing.T) {
	deleted := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		}
	})
	if err := c.DeleteRepoNotification(context.Background(), "myorg/myrepo", "abc-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("DELETE was never called")
	}
}

// TestGetRepoNotification_NotFound verifies that a uuid absent from the list returns IsNotFound.
func TestGetRepoNotification_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"notifications": []any{}})
	})
	_, err := c.GetRepoNotification(context.Background(), "myorg/myrepo", "missing")
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

// TestCreateRepoNotification_VulnerabilityLevelSentAsInt32 asserts that the level integer reaches the API unchanged.
func TestCreateRepoNotification_VulnerabilityLevelSentAsInt32(t *testing.T) {
	var capturedBody map[string]any

	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusCreated, client.Notification{
				UUID:        "abc-123",
				Event:       "vulnerability_found",
				Method:      "webhook",
				Config:      map[string]any{"url": "https://example.com"},
				EventConfig: map[string]any{"level": float64(1)},
			})
		}
	})

	_, err := c.CreateRepoNotification(context.Background(), "myorg/myrepo", client.CreateNotificationRequest{
		Event:       "vulnerability_found",
		Method:      "webhook",
		Config:      map[string]any{"url": "https://example.com"},
		EventConfig: map[string]any{"level": int32(1)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	eventConfig, _ := capturedBody["eventConfig"].(map[string]any)
	if eventConfig == nil {
		t.Fatal("request body missing eventConfig")
	}
	rawLevel := eventConfig["level"]
	switch v := rawLevel.(type) {
	case float64:
		if v != 1 {
			t.Errorf("eventConfig.level: want 1, got %v", v)
		}
	case string:
		t.Errorf("eventConfig.level was sent as a string %q; Quay expects an integer", v)
	default:
		t.Errorf("eventConfig.level: unexpected type %T with value %v", rawLevel, rawLevel)
	}
}

// TestEventConfig_VulnerabilityLevelString asserts the canonical severity name is returned
// for each index, and ("", false) when the key is absent or the index is out of range.
func TestEventConfig_VulnerabilityLevelString(t *testing.T) {
	tests := []struct {
		name    string
		config  client.EventConfig
		wantStr string
		wantOK  bool
	}{
		{"critical", client.EventConfig{"level": float64(0)}, "Critical", true},
		{"high", client.EventConfig{"level": float64(1)}, "High", true},
		{"medium", client.EventConfig{"level": float64(2)}, "Medium", true},
		{"low", client.EventConfig{"level": float64(3)}, "Low", true},
		{"negligible", client.EventConfig{"level": float64(4)}, "Negligible", true},
		{"unknown", client.EventConfig{"level": float64(5)}, "Unknown", true},
		{"absent", client.EventConfig{}, "", false},
		{"nil value", client.EventConfig{"level": nil}, "", false},
		{"out of range", client.EventConfig{"level": float64(99)}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.config.VulnerabilityLevelString()
			if ok != tt.wantOK {
				t.Fatalf("VulnerabilityLevelString() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.wantStr {
				t.Errorf("VulnerabilityLevelString() = %q, want %q", got, tt.wantStr)
			}
		})
	}
}

// TestVulnerabilityLevelIndexFromString asserts the correct integer index is returned
// for each canonical severity name, and (0, false) for unrecognised strings.
func TestVulnerabilityLevelIndexFromString(t *testing.T) {
	tests := []struct {
		input     string
		wantIndex int32
		wantOK    bool
	}{
		{"Critical", 0, true},
		{"High", 1, true},
		{"Medium", 2, true},
		{"Low", 3, true},
		{"Negligible", 4, true},
		{"Unknown", 5, true},
		{"critical", 0, false}, // case-sensitive
		{"", 0, false},
		{"Bogus", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := client.VulnerabilityLevelIndexFromString(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("VulnerabilityLevelIndexFromString(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && got != tt.wantIndex {
				t.Errorf("VulnerabilityLevelIndexFromString(%q) = %d, want %d", tt.input, got, tt.wantIndex)
			}
		})
	}
}

// TestEventConfig_VulnerabilityLevelInt32 asserts the level is returned as int32 and false when absent.
func TestEventConfig_VulnerabilityLevelInt32(t *testing.T) {
	tests := []struct {
		name      string
		config    client.EventConfig
		wantLevel int32
		wantOK    bool
	}{
		{"critical", client.EventConfig{"level": float64(0)}, 0, true},
		{"high", client.EventConfig{"level": float64(1)}, 1, true},
		{"medium", client.EventConfig{"level": float64(2)}, 2, true},
		{"low", client.EventConfig{"level": float64(3)}, 3, true},
		{"negligible", client.EventConfig{"level": float64(4)}, 4, true},
		{"unknown", client.EventConfig{"level": float64(5)}, 5, true},
		{"absent", client.EventConfig{}, 0, false},
		{"nil value", client.EventConfig{"level": nil}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.config.VulnerabilityLevelInt32()
			if ok != tt.wantOK {
				t.Fatalf("VulnerabilityLevelInt32() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.wantLevel {
				t.Errorf("VulnerabilityLevelInt32() = %d, want %d", got, tt.wantLevel)
			}
		})
	}
}
