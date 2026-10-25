package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/quay/terraform-provider-quay/internal/client"
)

var _ datasource.DataSource = &OrganizationDataSource{}

type OrganizationDataSource struct {
	client *client.Client
}

type OrganizationDataSourceModel struct {
	Name                types.String `tfsdk:"name"`
	Email               types.String `tfsdk:"email"`
	InvoiceEmail        types.Bool   `tfsdk:"invoice_email"`
	InvoiceEmailAddress types.String `tfsdk:"invoice_email_address"`
	TagExpirationS      types.Int64  `tfsdk:"tag_expiration_s"`
	IsFreeAccount       types.Bool   `tfsdk:"is_free_account"`
}

func NewOrganizationDataSource() datasource.DataSource {
	return &OrganizationDataSource{}
}

func (d *OrganizationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (d *OrganizationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches information about a Quay organization.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the organization.",
			},
			"email": schema.StringAttribute{
				Computed:    true,
				Description: "The contact email address for the organization.",
			},
			"invoice_email": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the organization receives invoice emails.",
			},
			"invoice_email_address": schema.StringAttribute{
				Computed:    true,
				Description: "The invoice email address.",
			},
			"tag_expiration_s": schema.Int64Attribute{
				Computed:    true,
				Description: "The default tag expiration in seconds.",
			},
			"is_free_account": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the organization is on a free plan.",
			},
		},
	}
}

func (d *OrganizationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *OrganizationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state OrganizationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	org, err := d.client.GetOrganization(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Organization", err.Error())
		return
	}

	state.Name = types.StringValue(org.Name)
	state.Email = types.StringValue(org.Email)
	if org.InvoiceEmail != nil {
		state.InvoiceEmail = types.BoolValue(*org.InvoiceEmail)
	} else {
		state.InvoiceEmail = types.BoolNull()
	}
	if org.InvoiceEmailAddress != nil {
		state.InvoiceEmailAddress = types.StringValue(*org.InvoiceEmailAddress)
	} else {
		state.InvoiceEmailAddress = types.StringNull()
	}
	if org.TagExpirationS != nil {
		state.TagExpirationS = types.Int64Value(int64(*org.TagExpirationS))
	} else {
		state.TagExpirationS = types.Int64Null()
	}
	if org.IsFreeAccount != nil {
		state.IsFreeAccount = types.BoolValue(*org.IsFreeAccount)
	} else {
		state.IsFreeAccount = types.BoolNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
