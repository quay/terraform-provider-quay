package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	provider "github.com/quay/terraform-provider-quay/internal/provider"
)

// importNotificationState calls ImportState and returns the resulting state response.
func importNotificationState(t *testing.T, importID string) *fwresource.ImportStateResponse {
	t.Helper()
	ctx := context.Background()

	r := provider.NewRepositoryNotificationResource()

	var schemaResp fwresource.SchemaResponse
	r.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	s := schemaResp.Schema

	req := fwresource.ImportStateRequest{ID: importID}
	resp := &fwresource.ImportStateResponse{
		State: tfsdk.State{
			Raw:    tftypes.NewValue(s.Type().TerraformType(ctx), nil),
			Schema: s,
		},
	}
	r.(fwresource.ResourceWithImportState).ImportState(ctx, req, resp)
	return resp
}

// getStringAttr reads a string attribute from an ImportStateResponse.
func getStringAttr(t *testing.T, resp *fwresource.ImportStateResponse, attr string) string {
	t.Helper()
	ctx := context.Background()
	var v types.String
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root(attr), &v)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("reading %q from import state: %s", attr, resp.Diagnostics)
	}
	return v.ValueString()
}

// TestRepositoryNotificationImport_SetsCorrectAttributes verifies that the 3-part
// import ID is parsed and all attributes are set correctly.
func TestRepositoryNotificationImport_SetsCorrectAttributes(t *testing.T) {
	const (
		namespace = "myorg"
		repoName  = "myrepo"
		uuid      = "32bb0ef6-ad69-4ccb-b64e-ddbc8bfd444d"
		importID  = namespace + "/" + repoName + "/" + uuid
		wantID    = namespace + "/" + repoName + "/" + uuid
	)

	resp := importNotificationState(t, importID)

	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState returned unexpected errors: %s", resp.Diagnostics)
	}

	if got := getStringAttr(t, resp, "namespace"); got != namespace {
		t.Errorf("namespace: want %q, got %q", namespace, got)
	}
	if got := getStringAttr(t, resp, "name"); got != repoName {
		t.Errorf("name: want %q, got %q", repoName, got)
	}
	if got := getStringAttr(t, resp, "uuid"); got != uuid {
		t.Errorf("uuid: want %q, got %q", uuid, got)
	}
	if got := getStringAttr(t, resp, "id"); got != wantID {
		t.Errorf("id: want %q, got %q", wantID, got)
	}
}

// TestRepositoryNotificationImport_IDDerivedFromParts verifies that id is derived from
// the parsed parts rather than passed through verbatim from the raw import ID.
func TestRepositoryNotificationImport_IDDerivedFromParts(t *testing.T) {
	const importID = "myorg/myrepo/32bb0ef6-ad69-4ccb-b64e-ddbc8bfd444d"

	resp := importNotificationState(t, importID)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState returned unexpected errors: %s", resp.Diagnostics)
	}

	gotNamespace := getStringAttr(t, resp, "namespace")
	gotName := getStringAttr(t, resp, "name")
	gotUUID := getStringAttr(t, resp, "uuid")
	gotID := getStringAttr(t, resp, "id")

	canonical := gotNamespace + "/" + gotName + "/" + gotUUID

	if gotID != canonical {
		t.Errorf("id %q does not equal namespace+\"/\"+name+\"/\"+uuid %q", gotID, canonical)
	}
}

// TestRepositoryNotificationImport_IDNotSetDirectlyFromReqID pins id == namespace+"/"+name+"/"+uuid across import ID shapes.
func TestRepositoryNotificationImport_IDNotSetDirectlyFromReqID(t *testing.T) {
	cases := []struct {
		name          string
		importID      string
		wantNamespace string
		wantName      string
		wantUUID      string
	}{
		{
			name:          "simple",
			importID:      "acme/frontend/aaaaaaaa-0000-0000-0000-000000000001",
			wantNamespace: "acme",
			wantName:      "frontend",
			wantUUID:      "aaaaaaaa-0000-0000-0000-000000000001",
		},
		{
			name:          "hyphenated org and repo",
			importID:      "my-org/my-repo/bbbbbbbb-0000-0000-0000-000000000002",
			wantNamespace: "my-org",
			wantName:      "my-repo",
			wantUUID:      "bbbbbbbb-0000-0000-0000-000000000002",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := importNotificationState(t, tc.importID)
			if resp.Diagnostics.HasError() {
				t.Fatalf("ImportState errors: %s", resp.Diagnostics)
			}

			gotNamespace := getStringAttr(t, resp, "namespace")
			gotName := getStringAttr(t, resp, "name")
			gotUUID := getStringAttr(t, resp, "uuid")
			gotID := getStringAttr(t, resp, "id")

			if gotNamespace != tc.wantNamespace {
				t.Errorf("namespace: want %q, got %q", tc.wantNamespace, gotNamespace)
			}
			if gotName != tc.wantName {
				t.Errorf("name: want %q, got %q", tc.wantName, gotName)
			}
			if gotUUID != tc.wantUUID {
				t.Errorf("uuid: want %q, got %q", tc.wantUUID, gotUUID)
			}

			wantID := tc.wantNamespace + "/" + tc.wantName + "/" + tc.wantUUID
			if gotID != wantID {
				t.Errorf("id: want %q, got %q", wantID, gotID)
			}
		})
	}
}
