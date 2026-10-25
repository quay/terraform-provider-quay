package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/quay/terraform-provider-quay/internal/client"
)

var _ resource.Resource = &OrganizationResource{}
var _ resource.ResourceWithImportState = &OrganizationResource{}

type OrganizationResource struct {
	client *client.Client
}

type OrganizationResourceModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	Email               types.String `tfsdk:"email"`
	InvoiceEmail        types.Bool   `tfsdk:"invoice_email"`
	InvoiceEmailAddress types.String `tfsdk:"invoice_email_address"`
	TagExpirationS      types.Int64  `tfsdk:"tag_expiration_s"`
}

func NewOrganizationResource() resource.Resource {
	return &OrganizationResource{}
}

func (r *OrganizationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (r *OrganizationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Quay organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The resource identifier (same as name).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the organization.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"email": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The contact email address for the organization.",
				Default:     stringdefault.StaticString(""),
			},
			"invoice_email": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the organization receives invoice emails.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"invoice_email_address": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The email address to send invoices to.",
				Default:     stringdefault.StaticString(""),
			},
			"tag_expiration_s": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Description: "How long deleted tags can be restored from the time machine, in seconds. " +
					"Must be one of Quay's TAG_EXPIRATION_OPTIONS: 0, 86400, 604800, 1209600, or 2419200. " +
					"Lowering this value shortens the restore window for tags already deleted.",
				Validators: []validator.Int64{
					int64validator.OneOf(0, 86400, 604800, 1209600, 2419200),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *OrganizationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OrganizationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	org, err := r.client.CreateOrganization(ctx, client.CreateOrganizationRequest{
		Name:  plan.Name.ValueString(),
		Email: plan.Email.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Organization", err.Error())
		return
	}

	// Quay's POST response omits invoice_email and tag_expiration_s, so the
	// follow-up PUT must use the plan values, not the API defaults.
	needsUpdate := (!plan.InvoiceEmail.IsNull() && !plan.InvoiceEmail.IsUnknown()) ||
		(!plan.InvoiceEmailAddress.IsNull() && !plan.InvoiceEmailAddress.IsUnknown()) ||
		(!plan.TagExpirationS.IsNull() && !plan.TagExpirationS.IsUnknown())
	var updateReq client.UpdateOrganizationRequest
	if needsUpdate {
		updateReq = client.UpdateOrganizationRequest{
			Email:               plan.Email.ValueString(),
			InvoiceEmail:        plan.InvoiceEmail.ValueBool(),
			InvoiceEmailAddress: plan.InvoiceEmailAddress.ValueString(),
			TagExpirationS:      int(plan.TagExpirationS.ValueInt64()),
		}
	}

	// A failure in the follow-up PUT would otherwise orphan the org with no state entry.
	r.mapToState(org, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if needsUpdate {
		org, err = r.client.UpdateOrganization(ctx, plan.Name.ValueString(), updateReq)
		if err != nil {
			resp.Diagnostics.AddError("Error Configuring Organization", err.Error())
			return
		}
		r.mapToState(org, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *OrganizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OrganizationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	org, err := r.client.GetOrganization(ctx, state.Name.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Organization", err.Error())
		return
	}

	r.mapToState(org, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrganizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OrganizationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	org, err := r.client.UpdateOrganization(ctx, plan.Name.ValueString(), client.UpdateOrganizationRequest{
		Email:               plan.Email.ValueString(),
		InvoiceEmail:        plan.InvoiceEmail.ValueBool(),
		InvoiceEmailAddress: plan.InvoiceEmailAddress.ValueString(),
		TagExpirationS:      int(plan.TagExpirationS.ValueInt64()),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Organization", err.Error())
		return
	}

	r.mapToState(org, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrganizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OrganizationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Quay deletes all org content immediately with no grace period, including
	// resources Terraform never managed. The guard covers image repositories only;
	// robots, teams and OAuth applications are not protected.
	hasRepos, err := r.client.HasOrganizationRepositories(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Checking Organization Repositories", err.Error())
		return
	}
	if hasRepos {
		resp.Diagnostics.AddError(
			"Cannot Delete Non-Empty Organization",
			fmt.Sprintf(
				"Organization %q still contains image repositories. Deleting it would "+
					"permanently destroy every repository, image, robot account, team and "+
					"OAuth application in the organization — including resources not managed "+
					"by Terraform — with no grace period.\n\n"+
					"Remove all repositories from the organization before destroying it. "+
					"Note that changing `name` also triggers a replace (delete + create) "+
					"which would delete all content. An organization that has only robots, "+
					"teams or OAuth applications (but no repositories) is not protected by "+
					"this guard.",
				state.Name.ValueString(),
			),
		)
		return
	}

	err = r.client.DeleteOrganization(ctx, state.Name.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error Deleting Organization", err.Error())
	}
}

func (r *OrganizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

func (r *OrganizationResource) mapToState(org *client.Organization, state *OrganizationResourceModel) {
	state.ID = types.StringValue(org.Name)
	state.Name = types.StringValue(org.Name)
	state.Email = types.StringValue(org.Email)
	if org.InvoiceEmail != nil {
		state.InvoiceEmail = types.BoolValue(*org.InvoiceEmail)
	}
	if org.InvoiceEmailAddress != nil {
		state.InvoiceEmailAddress = types.StringValue(*org.InvoiceEmailAddress)
	} else {
		state.InvoiceEmailAddress = types.StringValue("")
	}
	if org.TagExpirationS != nil {
		state.TagExpirationS = types.Int64Value(int64(*org.TagExpirationS))
	}
}
