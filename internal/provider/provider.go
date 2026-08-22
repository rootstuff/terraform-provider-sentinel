package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const defaultBaseURL = "https://sentinel.rootstuff.io/api/v1"

type sentinelProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &sentinelProvider{version: version}
	}
}

type sentinelProviderModel struct {
	BaseURL            types.String `tfsdk:"base_url"`
	APIToken           types.String `tfsdk:"api_token"`
	InsecureSkipVerify types.Bool   `tfsdk:"insecure_skip_verify"`
}

func (p *sentinelProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "sentinel"
	resp.Version = p.version
}

func (p *sentinelProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage Sentinel uptime monitoring resources (monitors, webhook endpoints) through the versioned Sentinel API.",
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Sentinel API token with the permissions the managed resources need (read plus create/update/delete). Falls back to the `SENTINEL_API_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"base_url": schema.StringAttribute{
				MarkdownDescription: "API base URL. Defaults to `" + defaultBaseURL + "`.",
				Optional:            true,
			},
			"insecure_skip_verify": schema.BoolAttribute{
				MarkdownDescription: "Skip TLS certificate verification. Only for local development against a self-signed instance.",
				Optional:            true,
			},
		},
	}
}

func (p *sentinelProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config sentinelProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token := os.Getenv("SENTINEL_API_TOKEN")
	if !config.APIToken.IsNull() {
		token = config.APIToken.ValueString()
	}
	if token == "" {
		resp.Diagnostics.AddError(
			"Missing Sentinel API token",
			"Set the provider's api_token attribute or the SENTINEL_API_TOKEN environment variable. Tokens are created on the API settings page.",
		)

		return
	}

	baseURL := defaultBaseURL
	if !config.BaseURL.IsNull() {
		baseURL = config.BaseURL.ValueString()
	}

	client := newAPIClient(baseURL, token, config.InsecureSkipVerify.ValueBool())
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *sentinelProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewMonitorResource,
		NewGroupResource,
		NewWebhookEndpointResource,
	}
}

func (p *sentinelProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
