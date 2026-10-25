package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/quay/terraform-provider-quay/internal/client"
)

var _ resource.Resource = &RepositoryResource{}
var _ resource.ResourceWithImportState = &RepositoryResource{}

type RepositoryResource struct {
	client *client.Client
}

type RepositoryResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Namespace   types.String `tfsdk:"namespace"`
	Name        types.String `tfsdk:"name"`
	Visibility  types.String `tfsdk:"visibility"`
	Description types.String `tfsdk:"description"`
}

func NewRepositoryResource() resource.Resource {
	return &RepositoryResource{}
}

func (r *RepositoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_repository"
}

func (r *RepositoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Quay repository.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The resource identifier (namespace/name).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"namespace": schema.StringAttribute{
				Required:    true,
				Description: "The namespace (organization or username) that owns the repository.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the repository.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"visibility": schema.StringAttribute{
				Required:    true,
				Description: "The visibility of the repository. One of: public, private.",
				Validators: []validator.String{
					stringvalidator.OneOf("public", "private"),
				},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "A markdown description of the repository.",
				Default:     stringdefault.StaticString(""),
			},
		},
	}
}

func (r *RepositoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *RepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RepositoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repo, err := r.client.CreateRepository(ctx, client.CreateRepositoryRequest{
		Repository:  plan.Name.ValueString(),
		Namespace:   plan.Namespace.ValueString(),
		Visibility:  plan.Visibility.ValueString(),
		Description: plan.Description.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Repository", err.Error())
		return
	}

	r.mapToState(repo, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repo, err := r.client.GetRepository(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		// Quay returns 403 when a repo's org namespace is deleted out-of-band.
		// For user namespaces, GetOrganization always returns 404, so the check
		// must also consult the user endpoint to avoid treating a permission
		// error as a deleted namespace.
		if client.IsForbidden(err) {
			exists, nsErr := r.client.NamespaceExists(ctx, state.Namespace.ValueString())
			if nsErr == nil && !exists {
				resp.State.RemoveResource(ctx)
				return
			}
		}
		resp.Diagnostics.AddError("Error Reading Repository", err.Error())
		return
	}

	r.mapToState(repo, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *RepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan RepositoryResourceModel
	var state RepositoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repoPath := state.ID.ValueString()

	repo, err := r.client.UpdateRepository(ctx, repoPath, client.UpdateRepositoryRequest{
		Description: plan.Description.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Repository", err.Error())
		return
	}

	if plan.Visibility.ValueString() != state.Visibility.ValueString() {
		if err := r.client.ChangeRepositoryVisibility(ctx, repoPath, client.ChangeVisibilityRequest{
			Visibility: plan.Visibility.ValueString(),
		}); err != nil {
			// The description was already updated; state must reflect what Quay actually has.
			repo.IsPublic = state.Visibility.ValueString() == client.VisibilityPublic
			r.mapToState(repo, &plan)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Error Changing Repository Visibility", err.Error())
			return
		}
		repo.IsPublic = plan.Visibility.ValueString() == client.VisibilityPublic
	}

	r.mapToState(repo, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RepositoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Quay purges tag content immediately with no grace period.
	hasTags, err := r.client.HasRepositoryTags(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Checking Repository Tags", err.Error())
		return
	}
	if hasTags {
		resp.Diagnostics.AddError(
			"Cannot Delete Non-Empty Repository",
			fmt.Sprintf(
				"Repository %q contains active tags and images. Deleting it would "+
					"permanently destroy all content with no grace period.\n\n"+
					"Remove all tags from the repository before destroying it. "+
					"Note that changing `name` or `namespace` also triggers a replace "+
					"(delete + create) which would delete all content. Deleted tags "+
					"that are still within the restore window are not protected by "+
					"this guard and will be lost on deletion.",
				state.ID.ValueString(),
			),
		)
		return
	}

	err = r.client.DeleteRepository(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error Deleting Repository", err.Error())
	}
}

func (r *RepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, diags := splitImportID(resp, req.ID, 2, "my-org/my-repo")
	if diags.HasError() {
		return
	}
	setAttrs(ctx, resp, map[string]string{
		"id":        req.ID,
		"namespace": parts[0],
		"name":      parts[1],
	})
}

func (r *RepositoryResource) mapToState(repo *client.Repository, state *RepositoryResourceModel) {
	state.Namespace = types.StringValue(repo.Namespace)
	state.Name = types.StringValue(repo.Name)
	state.Description = types.StringValue(repo.Description)
	if repo.IsPublic {
		state.Visibility = types.StringValue(client.VisibilityPublic)
	} else {
		state.Visibility = types.StringValue(client.VisibilityPrivate)
	}
	state.ID = types.StringValue(fmt.Sprintf("%s/%s", repo.Namespace, repo.Name))
}
