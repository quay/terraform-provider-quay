package client

import (
	"context"
	"fmt"
	"net/url"
)

// Visibility values accepted by the Quay API.
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

var validVisibilities = map[string]bool{
	VisibilityPublic:  true,
	VisibilityPrivate: true,
}

// Repository represents a Quay repository.
type Repository struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsPublic    bool   `json:"is_public"`
}

type CreateRepositoryRequest struct {
	Repository  string `json:"repository"`
	Namespace   string `json:"namespace,omitempty"`
	Visibility  string `json:"visibility"`
	Description string `json:"description"`
}

type UpdateRepositoryRequest struct {
	Description string `json:"description"`
}

type ChangeVisibilityRequest struct {
	Visibility string `json:"visibility"`
}

func (c *Client) CreateRepository(ctx context.Context, req CreateRepositoryRequest) (*Repository, error) {
	if err := c.post(ctx, "repository", req, nil); err != nil {
		return nil, fmt.Errorf("creating repository %q: %w", req.Repository, err)
	}
	// Quay's POST /repository response omits is_public, so the actual visibility must be fetched.
	repo, err := c.GetRepository(ctx, fmt.Sprintf("%s/%s", req.Namespace, req.Repository))
	if err != nil {
		return nil, fmt.Errorf("repository %q was already created but could not be read back: %w", req.Repository, err)
	}
	return repo, nil
}

func (c *Client) GetRepository(ctx context.Context, repository string) (*Repository, error) {
	var repo Repository
	if err := c.get(ctx, fmt.Sprintf("repository/%s?includeTags=false", repository), &repo); err != nil {
		return nil, fmt.Errorf("getting repository %q: %w", repository, err)
	}
	return &repo, nil
}

func (c *Client) UpdateRepository(ctx context.Context, repository string, req UpdateRepositoryRequest) (*Repository, error) {
	if err := c.put(ctx, fmt.Sprintf("repository/%s", repository), req, nil); err != nil {
		return nil, fmt.Errorf("updating repository %q: %w", repository, err)
	}
	return c.GetRepository(ctx, repository)
}

func (c *Client) ChangeRepositoryVisibility(ctx context.Context, repository string, req ChangeVisibilityRequest) error {
	if !validVisibilities[req.Visibility] {
		return fmt.Errorf("invalid visibility %q: must be %q or %q", req.Visibility, VisibilityPublic, VisibilityPrivate)
	}
	if err := c.post(ctx, fmt.Sprintf("repository/%s/changevisibility", repository), req, nil); err != nil {
		return fmt.Errorf("changing visibility of repository %q: %w", repository, err)
	}
	return nil
}

func (c *Client) DeleteRepository(ctx context.Context, repository string) error {
	if err := c.delete(ctx, fmt.Sprintf("repository/%s", repository)); err != nil {
		return fmt.Errorf("deleting repository %q: %w", repository, err)
	}
	return nil
}

// DeleteTag permanently deletes a tag from a repository.
func (c *Client) DeleteTag(ctx context.Context, repository, tag string) error {
	if err := c.delete(ctx, fmt.Sprintf("repository/%s/tag/%s", repository, tag)); err != nil {
		return fmt.Errorf("deleting tag %q from repository %q: %w", tag, repository, err)
	}
	return nil
}

// tagListResponse is the shape of GET repository/<ns>/<name>/tag/.
type tagListResponse struct {
	Tags []map[string]any `json:"tags"`
}

// HasRepositoryTags reports whether the repository has at least one active tag.
// Returns (false, nil) for a missing repository. Deleted tags within the
// tag_expiration_s restore window are not counted, so a repository that appears
// empty may still have restorable content that is permanently lost on deletion.
func (c *Client) HasRepositoryTags(ctx context.Context, repository string) (bool, error) {
	path := fmt.Sprintf("repository/%s/tag/?onlyActiveTags=true&limit=1", repository)
	var resp tagListResponse
	if err := c.get(ctx, path, &resp); err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("listing tags for repository %q: %w", repository, err)
	}
	return len(resp.Tags) > 0, nil
}

// NamespaceExists reports whether the given namespace exists as a Quay
// organisation or user account. Returns (false, nil) only when both lookups
// return 404.
//
// A 403 on a repository in a user namespace is a genuine permissions error,
// not a signal that the namespace is gone. Checking only the org endpoint
// misclassifies user-namespace repositories as deleted because GetOrganization
// always returns 404 for user accounts.
func (c *Client) NamespaceExists(ctx context.Context, name string) (bool, error) {
	err := c.get(ctx, fmt.Sprintf("organization/%s", name), nil)
	if err == nil {
		return true, nil
	}
	if !IsNotFound(err) {
		return false, err
	}

	err = c.get(ctx, fmt.Sprintf("users/%s", name), nil)
	if err == nil {
		return true, nil
	}
	if !IsNotFound(err) {
		return false, err
	}

	return false, nil
}

// repoListResponse is the shape of GET repository?namespace=<name>.
type repoListResponse struct {
	Repositories []map[string]any `json:"repositories"`
}

// HasOrganizationRepositories reports whether the organization contains at least
// one image repository. Robots, teams and OAuth applications are not counted
// because Quay's list endpoint returns image repositories only, so an org with
// those but no repos will be deleted along with them without triggering the guard.
func (c *Client) HasOrganizationRepositories(ctx context.Context, name string) (bool, error) {
	path := fmt.Sprintf("repository?namespace=%s", url.QueryEscape(name))
	var resp repoListResponse
	if err := c.get(ctx, path, &resp); err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("listing repositories for organization %q: %w", name, err)
	}
	return len(resp.Repositories) > 0, nil
}
