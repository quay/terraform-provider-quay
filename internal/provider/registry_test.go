package provider_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// registryBearerToken fetches a Docker V2 bearer token scoped to push+pull on the given repository.
// Quay's registry auth requires basic auth with the username and password, not the OAuth token.
func registryBearerToken(t *testing.T, repository string) string {
	t.Helper()

	tokenURL := fmt.Sprintf(
		"%s/v2/auth?service=localhost:8080&scope=repository:%s:push,pull",
		testState.quayURL,
		url.QueryEscape(repository),
	)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tokenURL, nil)
	if err != nil {
		t.Fatalf("building token request: %v", err)
	}
	req.SetBasicAuth(quayAdminUser, quayAdminPass)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetching registry token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("registry token endpoint returned %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decoding token response: %v", err)
	}
	return result.Token
}

// pushImageToQuay pushes a minimal scratch image into the test Quay instance via the Docker V2 HTTP API.
// The Docker daemon cannot be used because Quay's WWW-Authenticate realm advertises
// SERVER_HOSTNAME (localhost:8080), which differs from the mapped host port.
func pushImageToQuay(t *testing.T, repository, tag string) {
	t.Helper()

	bearerToken := registryBearerToken(t, repository)
	base := testState.quayURL
	ctx := context.Background()

	do := func(method, rawURL string, body []byte, contentType string) *http.Response {
		t.Helper()
		var r io.Reader
		if body != nil {
			r = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, rawURL, r)
		if err != nil {
			t.Fatalf("building request %s %s: %v", method, rawURL, err)
		}
		req.Header.Set("Authorization", "Bearer "+bearerToken)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, rawURL, err)
		}
		return resp
	}

	mustClose := func(r io.Closer) { _ = r.Close() }

	// Quay's Location headers use SERVER_HOSTNAME (localhost:8080), not the mapped host port.
	rewriteLocation := func(location string) string {
		parsed, err := url.Parse(location)
		if err != nil {
			return location
		}
		baseURL, err := url.Parse(base)
		if err != nil {
			return location
		}
		parsed.Scheme = baseURL.Scheme
		parsed.Host = baseURL.Host
		return parsed.String()
	}

	pushBlob := func(blob []byte) string {
		t.Helper()
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(blob))

		resp := do(http.MethodHead,
			fmt.Sprintf("%s/v2/%s/blobs/%s", base, repository, digest),
			nil, "")
		mustClose(resp.Body)
		if resp.StatusCode == http.StatusOK {
			return digest
		}

		resp = do(http.MethodPost,
			fmt.Sprintf("%s/v2/%s/blobs/uploads/", base, repository),
			nil, "")
		location := rewriteLocation(resp.Header.Get("Location"))
		mustClose(resp.Body)
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("POST blobs/uploads/ for %s: status %d", repository, resp.StatusCode)
		}
		if location == "" {
			t.Fatalf("POST blobs/uploads/ returned no Location header")
		}

		sep := "?"
		if strings.ContainsRune(location, '?') {
			sep = "&"
		}
		resp = do(http.MethodPut, location+sep+"digest="+digest, blob, "application/octet-stream")
		body, _ := io.ReadAll(resp.Body)
		mustClose(resp.Body)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("PUT blob for %s: status %d: %s", repository, resp.StatusCode, body)
		}
		return digest
	}

	layerDigest := pushBlob([]byte{})
	configJSON := []byte(`{"history":[{}],"rootfs":{"type":"layers"}}`)
	configDigest := pushBlob(configJSON)

	manifest, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]any{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size":      len(configJSON),
			"digest":    configDigest,
		},
		"layers": []map[string]any{{
			"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			"size":      0,
			"digest":    layerDigest,
		}},
	})
	if err != nil {
		t.Fatalf("marshalling manifest: %v", err)
	}

	resp := do(http.MethodPut,
		fmt.Sprintf("%s/v2/%s/manifests/%s", base, repository, tag),
		manifest,
		"application/vnd.docker.distribution.manifest.v2+json",
	)
	respBody, _ := io.ReadAll(resp.Body)
	mustClose(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT manifest for %s:%s: status %d: %s", repository, tag, resp.StatusCode, respBody)
	}
}
