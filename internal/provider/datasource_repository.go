package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/quay/terraform-provider-quay/internal/client"
)

var _ datasource.DataSource = &RepositoryDataSource{}

type RepositoryDataSource struct {
	client *client.Client
}

type RepositoryDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Namespace   types.String `tfsdk:"namespace"`
	Name        types.String `tfsdk:"name"`
	Visibility  types.String `tfsdk:"visibility"`
	Description types.String `tfsdk:"description"`
}

func NewRepositoryDataSource() datasource.DataSource {
	return &RepositoryDataSource{}
}

func (d *RepositoryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_repository"
}

func (d *RepositoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches information about a Quay repository.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The resource identifier (namespace/name).",
			},
			"namespace": schema.StringAttribute{
				Required:    true,
				Description: "The namespace (organization or username) that owns the repository.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the repository.",
			},
			"visibility": schema.StringAttribute{
				Computed:    true,
				Description: "The visibility of the repository: public or private.",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "The markdown description of the repository.",
			},
		},
	}
}

func (d *RepositoryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RepositoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state RepositoryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repoPath := fmt.Sprintf("%s/%s", state.Namespace.ValueString(), state.Name.ValueString())
	repo, err := d.client.GetRepository(ctx, repoPath)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Repository", err.Error())
		return
	}

	state.ID = types.StringValue(repoPath)
	state.Namespace = types.StringValue(repo.Namespace)
	state.Name = types.StringValue(repo.Name)
	state.Description = types.StringValue(repo.Description)
	if repo.IsPublic {
		state.Visibility = types.StringValue(client.VisibilityPublic)
	} else {
		state.Visibility = types.StringValue(client.VisibilityPrivate)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
