package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/quay/terraform-provider-quay/internal/client"
)

var _ provider.Provider = &QuayProvider{}

type QuayProvider struct {
	version string
}

type QuayProviderModel struct {
	URL     types.String `tfsdk:"url"`
	Token   types.String `tfsdk:"token"`
	Timeout types.Int64  `tfsdk:"timeout"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &QuayProvider{version: version}
	}
}

func (p *QuayProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "quay"
	resp.Version = p.version
}

func (p *QuayProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interact with Quay.io or a self-hosted Red Hat Quay instance.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional:    true,
				Description: "The base URL of the Quay instance. Defaults to https://quay.io. Can also be set via the QUAY_URL environment variable.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "OAuth bearer token for authenticating with the Quay API. Can also be set via the QUAY_TOKEN environment variable.",
			},
			"timeout": schema.Int64Attribute{
				Optional:    true,
				Description: fmt.Sprintf("HTTP client timeout in seconds. Defaults to %d. Set to 0 to disable the timeout. Can also be set via the QUAY_TIMEOUT environment variable.", int(client.DefaultTimeout.Seconds())),
			},
		},
	}
}

func (p *QuayProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config QuayProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.URL.IsUnknown() {
		resp.Diagnostics.AddError(
			"Invalid Quay URL",
			"The `url` provider attribute is unknown. Ensure it resolves to a known value before configuring the provider.",
		)
		return
	}
	if config.Token.IsUnknown() {
		resp.Diagnostics.AddError(
			"Invalid Quay API Token",
			"The `token` provider attribute is unknown. Ensure it resolves to a known value before configuring the provider.",
		)
		return
	}

	url := os.Getenv("QUAY_URL")
	if !config.URL.IsNull() {
		if v := config.URL.ValueString(); v != "" {
			url = v
		}
	}
	if err := client.ValidateURL(url); err != nil {
		resp.Diagnostics.AddError("Invalid Quay URL", err.Error())
		return
	}

	token := os.Getenv("QUAY_TOKEN")
	if !config.Token.IsNull() {
		if v := config.Token.ValueString(); v != "" {
			token = v
		}
	}

	if token == "" {
		resp.Diagnostics.AddError(
			"Missing Quay API Token",
			"A Quay API token must be provided via the `token` provider attribute or the QUAY_TOKEN environment variable.",
		)
		return
	}

	timeout := client.DefaultTimeout
	if raw := os.Getenv("QUAY_TIMEOUT"); raw != "" {
		secs, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid QUAY_TIMEOUT",
				fmt.Sprintf("QUAY_TIMEOUT must be an integer number of seconds, got %q: %s", raw, err),
			)
			return
		}
		if secs < 0 {
			resp.Diagnostics.AddError(
				"Invalid QUAY_TIMEOUT",
				fmt.Sprintf("QUAY_TIMEOUT must be a non-negative integer; got %d. Use 0 to disable the timeout.", secs),
			)
			return
		}
		timeout = time.Duration(secs) * time.Second
	}
	if !config.Timeout.IsNull() && !config.Timeout.IsUnknown() {
		secs := config.Timeout.ValueInt64()
		if secs < 0 {
			resp.Diagnostics.AddError(
				"Invalid Timeout",
				fmt.Sprintf("The `timeout` provider attribute must be a non-negative integer; got %d. Use 0 to disable the timeout.", secs),
			)
			return
		}
		timeout = time.Duration(secs) * time.Second
	}

	c := client.New(url, token, timeout)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *QuayProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewOrganizationResource,
		NewRepositoryResource,
		NewRepositoryNotificationResource,
	}
}

func (p *QuayProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewOrganizationDataSource,
		NewRepositoryDataSource,
	}
}

func (p *QuayProvider) Functions(_ context.Context) []func() function.Function {
	return nil
}
