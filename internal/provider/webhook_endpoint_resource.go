package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

/*
sentinel_webhook_endpoint maps to /api/v1/webhook-endpoints. The API treats
url, auth_token, and signing_secret as write-only (webhook URLs routinely
carry credentials), so those attributes live in state from configuration and
are never refreshed: Read updates only the non-secret fields. After a
`terraform import`, the secret attributes are empty in state and the first
apply resends them from configuration, which the API treats as an overwrite
with the same values.
*/

func NewWebhookEndpointResource() resource.Resource {
	return &webhookEndpointResource{}
}

type webhookEndpointResource struct {
	client *apiClient
}

type webhookEndpointResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	URL            types.String `tfsdk:"url"`
	URLHost        types.String `tfsdk:"url_host"`
	AuthType       types.String `tfsdk:"auth_type"`
	AuthToken      types.String `tfsdk:"auth_token"`
	AuthHeaderName types.String `tfsdk:"auth_header_name"`
	SigningSecret  types.String `tfsdk:"signing_secret"`
	Severities     types.Set    `tfsdk:"severities"`
	IsActive       types.Bool   `tfsdk:"is_active"`
}

func (r *webhookEndpointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook_endpoint"
}

func (r *webhookEndpointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An outbound webhook destination for team alerts (an incident tool, an internal automation, a partner intake).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name, max 100 characters.",
				Required:            true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "HTTPS destination URL. Write-only: the API never returns it (webhook URLs routinely carry secrets), so drift on this attribute is not detected.",
				Required:            true,
				Sensitive:           true,
			},
			"url_host": schema.StringAttribute{
				MarkdownDescription: "Host part of the configured URL, as reported by the API.",
				Computed:            true,
			},
			"auth_type": schema.StringAttribute{
				MarkdownDescription: "`none`, `bearer`, `basic`, or `header`. Defaults to `none`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("none"),
			},
			"auth_token": schema.StringAttribute{
				MarkdownDescription: "Credential for bearer/basic/header auth. Write-only, never returned by the API.",
				Optional:            true,
				Sensitive:           true,
			},
			"auth_header_name": schema.StringAttribute{
				MarkdownDescription: "Header name carrying the token when `auth_type` is `header`.",
				Optional:            true,
			},
			"signing_secret": schema.StringAttribute{
				MarkdownDescription: "Optional HMAC signing secret (16 to 255 characters). Write-only.",
				Optional:            true,
				Sensitive:           true,
			},
			"severities": schema.SetAttribute{
				MarkdownDescription: "Severities delivered to this endpoint: `critical`, `warning`, `info`. Defaults to all three.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"is_active": schema.BoolAttribute{
				MarkdownDescription: "Inactive endpoints are kept but receive nothing. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
		},
	}
}

func (r *webhookEndpointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*apiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *apiClient, got %T", req.ProviderData))

		return
	}

	r.client = client
}

func (r *webhookEndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.payloadFrom(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.do("POST", "/webhook-endpoints", payload)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create webhook endpoint", err.Error())

		return
	}

	r.applyResponse(ctx, created, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *webhookEndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := r.client.do("GET", "/webhook-endpoints/"+state.ID.ValueString(), nil)
	if errors.Is(err, errNotFound) {
		resp.State.RemoveResource(ctx)

		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read webhook endpoint", err.Error())

		return
	}

	r.applyResponse(ctx, endpoint, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *webhookEndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webhookEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.payloadFrom(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.do("PUT", "/webhook-endpoints/"+plan.ID.ValueString(), payload)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update webhook endpoint", err.Error())

		return
	}

	r.applyResponse(ctx, updated, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *webhookEndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.do("DELETE", "/webhook-endpoints/"+state.ID.ValueString(), nil)
	if err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Failed to delete webhook endpoint", err.Error())
	}
}

func (r *webhookEndpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *webhookEndpointResource) payloadFrom(ctx context.Context, plan webhookEndpointResourceModel, diags *diag.Diagnostics) map[string]any {
	payload := map[string]any{
		"name":      plan.Name.ValueString(),
		"url":       plan.URL.ValueString(),
		"auth_type": plan.AuthType.ValueString(),
	}

	if !plan.AuthToken.IsNull() {
		payload["auth_token"] = plan.AuthToken.ValueString()
	}
	if !plan.AuthHeaderName.IsNull() {
		payload["auth_header_name"] = plan.AuthHeaderName.ValueString()
	}
	if !plan.SigningSecret.IsNull() {
		payload["signing_secret"] = plan.SigningSecret.ValueString()
	}
	if !plan.IsActive.IsNull() && !plan.IsActive.IsUnknown() {
		payload["is_active"] = plan.IsActive.ValueBool()
	}
	if !plan.Severities.IsNull() && !plan.Severities.IsUnknown() {
		var severities []string
		diags.Append(plan.Severities.ElementsAs(ctx, &severities, false)...)
		payload["severities"] = severities
	}

	return payload
}

// applyResponse maps the API's non-secret fields onto the model. The secret
// attributes (url, auth_token, signing_secret) are left exactly as the plan
// or prior state holds them.
func (r *webhookEndpointResource) applyResponse(ctx context.Context, endpoint map[string]any, model *webhookEndpointResourceModel, diags *diag.Diagnostics) {
	if id, ok := fieldInt(endpoint, "id"); ok {
		model.ID = types.StringValue(fmt.Sprintf("%d", id))
	}
	if name, ok := fieldString(endpoint, "name"); ok {
		model.Name = types.StringValue(name)
	}
	if host, ok := fieldString(endpoint, "url_host"); ok {
		model.URLHost = types.StringValue(host)
	} else {
		model.URLHost = types.StringNull()
	}
	if authType, ok := fieldString(endpoint, "auth_type"); ok {
		model.AuthType = types.StringValue(authType)
	}
	if headerName, ok := fieldString(endpoint, "auth_header_name"); ok {
		model.AuthHeaderName = types.StringValue(headerName)
	} else {
		model.AuthHeaderName = types.StringNull()
	}
	if active, ok := fieldBool(endpoint, "is_active"); ok {
		model.IsActive = types.BoolValue(active)
	}
	if severities, ok := fieldStringSlice(endpoint, "severities"); ok {
		value, valueDiags := types.SetValueFrom(ctx, types.StringType, severities)
		diags.Append(valueDiags...)
		model.Severities = value
	}
}
