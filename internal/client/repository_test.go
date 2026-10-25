package client_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/quay/terraform-provider-quay/internal/client"
)

func TestCreateRepository(t *testing.T) {
	created := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/repository"):
			created = true
			writeJSON(w, http.StatusCreated, map[string]string{
				"namespace": "myorg",
				"name":      "myrepo",
				"kind":      "image",
			})
		case r.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, client.Repository{
				Namespace: "myorg",
				Name:      "myrepo",
				IsPublic:  false,
			})
		default:
			http.NotFound(w, r)
		}
	})

	repo, err := c.CreateRepository(context.Background(), client.CreateRepositoryRequest{
		Repository:  "myrepo",
		Namespace:   "myorg",
		Visibility:  "private",
		Description: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("POST /repository was never called")
	}
	if repo.Name != "myrepo" {
		t.Errorf("expected name myrepo, got %q", repo.Name)
	}
	if repo.IsPublic {
		t.Error("expected IsPublic=false")
	}
}

func TestGetRepository_DoesNotFetchTags(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("includeTags"); got != "false" {
			t.Errorf("includeTags: want %q, got %q", "false", got)
		}
		writeJSON(w, http.StatusOK, client.Repository{Namespace: "myorg", Name: "myrepo"})
	})
	if _, err := c.GetRepository(context.Background(), "myorg/myrepo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetRepository_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	_, err := c.GetRepository(context.Background(), "myorg/missing")
	if !client.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestUpdateRepository(t *testing.T) {
	updated := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			updated = true
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			writeJSON(w, http.StatusOK, client.Repository{
				Namespace:   "myorg",
				Name:        "myrepo",
				Description: "updated desc",
			})
		}
	})
	repo, err := c.UpdateRepository(context.Background(), "myorg/myrepo", client.UpdateRepositoryRequest{
		Description: "updated desc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Error("PUT was never called")
	}
	if repo.Description != "updated desc" {
		t.Errorf("expected description 'updated desc', got %q", repo.Description)
	}
}

func TestChangeRepositoryVisibility(t *testing.T) {
	changed := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "changevisibility") {
			changed = true
			w.WriteHeader(http.StatusOK)
		}
	})
	if err := c.ChangeRepositoryVisibility(context.Background(), "myorg/myrepo", client.ChangeVisibilityRequest{Visibility: "public"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("changevisibility endpoint was never called")
	}
}

func TestDeleteRepository(t *testing.T) {
	deleted := false
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		}
	})
	if err := c.DeleteRepository(context.Background(), "myorg/myrepo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("DELETE was never called")
	}
}

// ── HasRepositoryTags ──────────────────────────────────────────────────────

func TestHasRepositoryTags_NonEmpty(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/tag/") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("onlyActiveTags"); got != "true" {
			t.Errorf("onlyActiveTags: want %q, got %q", "true", got)
		}
		if got := r.URL.Query().Get("limit"); got != "1" {
			t.Errorf("limit: want %q, got %q", "1", got)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"tags": []map[string]any{{"name": "latest"}},
		})
	})

	hasTags, err := c.HasRepositoryTags(context.Background(), "myorg/myrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasTags {
		t.Error("want true for a repository with tags, got false")
	}
}

func TestHasRepositoryTags_Empty(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"tags": []any{}})
	})

	hasTags, err := c.HasRepositoryTags(context.Background(), "myorg/myrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasTags {
		t.Error("want false for a repository with no tags, got true")
	}
}

func TestHasRepositoryTags_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})

	hasTags, err := c.HasRepositoryTags(context.Background(), "myorg/missing")
	if err != nil {
		t.Fatalf("want (false, nil) for a missing repository, got error: %v", err)
	}
	if hasTags {
		t.Error("want false for a missing repository, got true")
	}
}

func TestHasRepositoryTags_QueryStringNotEscaped(t *testing.T) {
	var gotRawQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		writeJSON(w, http.StatusOK, map[string]any{"tags": []any{}})
	})

	_, _ = c.HasRepositoryTags(context.Background(), "myorg/myrepo")

	if !strings.Contains(gotRawQuery, "onlyActiveTags=true") {
		t.Errorf("want raw query to contain 'onlyActiveTags=true', got %q", gotRawQuery)
	}
}

// ── HasOrganizationRepositories ────────────────────────────────────────────

func TestHasOrganizationRepositories_NonEmpty(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("namespace"); got == "" {
			t.Error("want namespace query parameter, got none")
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"repositories": []map[string]any{{"name": "some-repo"}},
		})
	})

	hasRepos, err := c.HasOrganizationRepositories(context.Background(), "myorg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasRepos {
		t.Error("want true for an org with repositories, got false")
	}
}

func TestHasOrganizationRepositories_Empty(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"repositories": []any{}})
	})

	hasRepos, err := c.HasOrganizationRepositories(context.Background(), "myorg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasRepos {
		t.Error("want false for an org with no repositories, got true")
	}
}

func TestHasOrganizationRepositories_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})

	hasRepos, err := c.HasOrganizationRepositories(context.Background(), "missing-org")
	if err != nil {
		t.Fatalf("want (false, nil) for a missing org, got error: %v", err)
	}
	if hasRepos {
		t.Error("want false for a missing org, got true")
	}
}

// ----- NamespaceExists ---------------------------------------------------

// TestNamespaceExists_OrgNamespace verifies that an organisation namespace is recognised.
func TestNamespaceExists_OrgNamespace(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/organization/"):
			writeJSON(w, http.StatusOK, map[string]string{"name": "myorg"})
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
		}
	})
	exists, err := c.NamespaceExists(context.Background(), "myorg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected NamespaceExists=true for an org namespace, got false")
	}
}

// TestNamespaceExists_UserNamespace verifies that a user-account namespace is recognised.
// The original code only checked the org endpoint, so user namespaces always looked absent.
func TestNamespaceExists_UserNamespace(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/organization/"):
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
		case strings.HasPrefix(r.URL.Path, "/api/v1/users/"):
			writeJSON(w, http.StatusOK, map[string]string{"username": "alice"})
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
		}
	})
	exists, err := c.NamespaceExists(context.Background(), "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected NamespaceExists=true for a user namespace, got false")
	}
}

// TestNamespaceExists_Absent verifies that (false, nil) is returned when neither endpoint recognises the name.
func TestNamespaceExists_Absent(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "not found"})
	})
	exists, err := c.NamespaceExists(context.Background(), "vanished")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected NamespaceExists=false for an absent namespace, got true")
	}
}

func TestHasOrganizationRepositories_QueryStringNotEscaped(t *testing.T) {
	var gotRawQuery string
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		writeJSON(w, http.StatusOK, map[string]any{"repositories": []any{}})
	})

	_, _ = c.HasOrganizationRepositories(context.Background(), "myorg")

	if !strings.Contains(gotRawQuery, "namespace=myorg") {
		t.Errorf("want raw query to contain 'namespace=myorg', got %q", gotRawQuery)
	}
}
