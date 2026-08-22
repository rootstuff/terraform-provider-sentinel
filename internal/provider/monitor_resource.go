package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

/*
sentinel_monitor maps to /api/v1/monitors. The v1 field set is the one the
first customers actually manage (http/ping/port monitors with ssl, dns, and
domain sub-checks); the nested keyword/JSON/lighthouse settings are later
additions. monitored_regions values are deliberately NOT validated here:
region identifiers are due to be renamed server-side, and a released
provider must not reject the new names.
*/

func NewMonitorResource() resource.Resource {
	return &monitorResource{}
}

type monitorResource struct {
	client *apiClient
}

type monitorResourceModel struct {
	ID                    types.String  `tfsdk:"id"`
	URL                   types.String  `tfsdk:"url"`
	MonitorType           types.String  `tfsdk:"monitor_type"`
	Port                  types.Int64   `tfsdk:"port"`
	FriendlyName          types.String  `tfsdk:"friendly_name"`
	CheckInterval         types.Float64 `tfsdk:"check_interval"`
	CheckTypes            types.Set    `tfsdk:"check_types"`
	MonitoredRegions      types.Set    `tfsdk:"monitored_regions"`
	SSLExpiryThreshold    types.Int64   `tfsdk:"ssl_expiry_threshold"`
	DomainExpiryThreshold types.Int64   `tfsdk:"domain_expiry_threshold"`
	RequestTimeout        types.Int64   `tfsdk:"request_timeout"`
	FollowRedirects       types.Bool    `tfsdk:"follow_redirects"`
	ErrorTextDetection    types.Bool    `tfsdk:"error_text_detection"`
	HTTPMethod            types.String  `tfsdk:"http_method"`
	RequestHeaders        types.Map     `tfsdk:"request_headers"`
	RequestBody           types.String  `tfsdk:"request_body"`
	AcceptedStatusCodes   types.Set     `tfsdk:"accepted_status_codes"`
	SlowResponseThreshold types.Int64   `tfsdk:"slow_response_threshold"`
	HeartbeatInterval     types.Int64   `tfsdk:"heartbeat_interval"`
	HeartbeatCron         types.String  `tfsdk:"heartbeat_cron_expression"`
	HeartbeatTimezone     types.String  `tfsdk:"heartbeat_timezone"`
	HeartbeatGrace        types.Int64   `tfsdk:"heartbeat_grace"`
	PingURL               types.String  `tfsdk:"ping_url"`
	Status                types.String  `tfsdk:"status"`
	GroupID               types.Int64   `tfsdk:"group_id"`
	AuthType              types.String  `tfsdk:"auth_type"`
	AuthUsername          types.String  `tfsdk:"auth_username"`
	AuthPassword          types.String  `tfsdk:"auth_password"`
	KeywordSettings       types.Object  `tfsdk:"keyword_settings"`
	JSONAssertionSettings types.Object  `tfsdk:"json_assertion_settings"`
}

// Nested attribute shapes for keyword_settings and json_assertion_settings.
// The attr.Type maps must match the schema below exactly; ObjectValueFrom
// validates against them when mirroring API responses into state.
type keywordEntryModel struct {
	Phrase        types.String `tfsdk:"phrase"`
	Mode          types.String `tfsdk:"mode"`
	CaseSensitive types.Bool   `tfsdk:"case_sensitive"`
}

type keywordSettingsModel struct {
	Keywords     []keywordEntryModel `tfsdk:"keywords"`
	SearchTarget types.String        `tfsdk:"search_target"`
}

type jsonAssertionEntryModel struct {
	Path     types.String `tfsdk:"path"`
	Operator types.String `tfsdk:"operator"`
	Value    types.String `tfsdk:"value"`
}

type jsonAssertionSettingsModel struct {
	Assertions []jsonAssertionEntryModel `tfsdk:"assertions"`
}

var keywordEntryAttrTypes = map[string]attr.Type{
	"phrase":         types.StringType,
	"mode":           types.StringType,
	"case_sensitive": types.BoolType,
}

var keywordSettingsAttrTypes = map[string]attr.Type{
	"keywords":      types.ListType{ElemType: types.ObjectType{AttrTypes: keywordEntryAttrTypes}},
	"search_target": types.StringType,
}

var jsonAssertionEntryAttrTypes = map[string]attr.Type{
	"path":     types.StringType,
	"operator": types.StringType,
	"value":    types.StringType,
}

var jsonAssertionSettingsAttrTypes = map[string]attr.Type{
	"assertions": types.ListType{ElemType: types.ObjectType{AttrTypes: jsonAssertionEntryAttrTypes}},
}

func (r *monitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor"
}

func (r *monitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An uptime monitor: an HTTP(S) URL, ping host, or TCP port checked from every monitored region.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The URL (http monitors) or host (ping/port monitors) to check. Omit for heartbeat/cron monitors, which are pinged inbound.",
				Optional:            true,
			},
			"monitor_type": schema.StringAttribute{
				MarkdownDescription: "`http`, `ping`, `port`, `heartbeat`, or `cron`. Defaults to `http`. Heartbeat/cron monitors receive pings instead of being probed; see `ping_url`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("http"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "TCP port to check. Required when `monitor_type` is `port`.",
				Optional:            true,
			},
			"friendly_name": schema.StringAttribute{
				MarkdownDescription: "Display name shown in the dashboard and alerts.",
				Optional:            true,
			},
			"check_interval": schema.Float64Attribute{
				MarkdownDescription: "Minutes between checks (0.5 to 60; plan limits apply). Defaults to the plan's interval.",
				Optional:            true,
				Computed:            true,
			},
			"check_types": schema.SetAttribute{
				MarkdownDescription: "Sub-checks to run: `ssl`, `dns`, `domain`, `keyword`, `json`, `lighthouse` (plan gated).",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"monitored_regions": schema.SetAttribute{
				MarkdownDescription: "Region identifiers to check from. Defaults to all active regions.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"ssl_expiry_threshold": schema.Int64Attribute{
				MarkdownDescription: "Days before SSL expiry to alert.",
				Optional:            true,
				Computed:            true,
			},
			"domain_expiry_threshold": schema.Int64Attribute{
				MarkdownDescription: "Days before domain expiry to alert. Domain expiry is tracked at the registrable domain, and one alert fires per team per domain.",
				Optional:            true,
				Computed:            true,
			},
			"request_timeout": schema.Int64Attribute{
				MarkdownDescription: "Per-check request timeout in seconds (1 to 60).",
				Optional:            true,
				Computed:            true,
			},
			"follow_redirects": schema.BoolAttribute{
				MarkdownDescription: "Follow HTTP redirects when checking.",
				Optional:            true,
				Computed:            true,
			},
			"error_text_detection": schema.BoolAttribute{
				MarkdownDescription: "Detect server error text (PHP/framework error spew) on otherwise-200 pages.",
				Optional:            true,
				Computed:            true,
			},
			"http_method": schema.StringAttribute{
				MarkdownDescription: "HTTP method for http monitors: GET (default), POST, PUT, PATCH, DELETE, HEAD, OPTIONS.",
				Optional:            true,
				Computed:            true,
			},
			"request_headers": schema.MapAttribute{
				MarkdownDescription: "Extra request headers sent with http checks.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"request_body": schema.StringAttribute{
				MarkdownDescription: "Request body for POST/PUT/PATCH checks (max 10000 chars).",
				Optional:            true,
			},
			"accepted_status_codes": schema.SetAttribute{
				MarkdownDescription: "Status codes counted as up: exact (`\"200\"`) or classes (`\"2xx\"`). Defaults to 2xx and 3xx.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"slow_response_threshold": schema.Int64Attribute{
				MarkdownDescription: "Alert when responses exceed this many milliseconds (100 to 60000).",
				Optional:            true,
			},
			"heartbeat_interval": schema.Int64Attribute{
				MarkdownDescription: "Expected seconds between pings. Required when `monitor_type` is `heartbeat`.",
				Optional:            true,
			},
			"heartbeat_cron_expression": schema.StringAttribute{
				MarkdownDescription: "Cron schedule the pings follow. Required when `monitor_type` is `cron`.",
				Optional:            true,
			},
			"heartbeat_timezone": schema.StringAttribute{
				MarkdownDescription: "Timezone for the cron schedule. Defaults server-side.",
				Optional:            true,
				Computed:            true,
			},
			"heartbeat_grace": schema.Int64Attribute{
				MarkdownDescription: "Seconds of grace after a missed ping before alerting.",
				Optional:            true,
				Computed:            true,
			},
			"ping_url": schema.StringAttribute{
				MarkdownDescription: "The unique inbound ping URL for heartbeat/cron monitors. Treat as a secret: anyone holding it can fake health.",
				Computed:            true,
				Sensitive:           true,
			},
			"group_id": schema.Int64Attribute{
				MarkdownDescription: "Id of the `sentinel_group` this monitor belongs to. Omit for ungrouped.",
				Optional:            true,
			},
			"auth_type": schema.StringAttribute{
				MarkdownDescription: "HTTP authentication for the check: `basic`, `bearer`, or `digest`. Omit for unauthenticated checks.",
				Optional:            true,
			},
			"auth_username": schema.StringAttribute{
				MarkdownDescription: "Username (or token, for bearer auth) sent with authenticated checks. Required with `auth_type`.",
				Optional:            true,
				Sensitive:           true,
			},
			"auth_password": schema.StringAttribute{
				MarkdownDescription: "Password sent with authenticated checks. Required with `auth_type`. Write-only: the API never returns it, so drift on this attribute is not detected.",
				Optional:            true,
				Sensitive:           true,
			},
			"keyword_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "Content assertions run against the response body. Supplying keywords implies the `keyword` check type (plan gated). Mutually exclusive with `json_assertion_settings`.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"keywords": schema.ListNestedAttribute{
						MarkdownDescription: "Up to 10 phrases to assert on.",
						Required:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"phrase": schema.StringAttribute{
									MarkdownDescription: "Text to look for, max 500 characters.",
									Required:            true,
								},
								"mode": schema.StringAttribute{
									MarkdownDescription: "`must_contain` or `must_not_contain`.",
									Required:            true,
								},
								"case_sensitive": schema.BoolAttribute{
									MarkdownDescription: "Match case exactly. Defaults to false.",
									Optional:            true,
								},
							},
						},
					},
					"search_target": schema.StringAttribute{
						MarkdownDescription: "`html` (raw markup) or `text` (rendered text). Defaults server-side.",
						Optional:            true,
					},
				},
			},
			"json_assertion_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "Assertions run against a JSON API response. Supplying assertions implies the `json` check type (plan gated). Mutually exclusive with `keyword_settings`.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"assertions": schema.ListNestedAttribute{
						MarkdownDescription: "Up to 10 assertions on response fields.",
						Required:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"path": schema.StringAttribute{
									MarkdownDescription: "Dot-notation path into the response, e.g. `status` or `queue.depth`.",
									Required:            true,
								},
								"operator": schema.StringAttribute{
									MarkdownDescription: "`equals`, `not_equals`, `contains`, `not_contains`, `exists`, `not_exists`, `gt`, `gte`, `lt`, `lte`, `regex`, `not_regex`.",
									Required:            true,
								},
								"value": schema.StringAttribute{
									MarkdownDescription: "Comparison value, always given as a string (numeric comparisons coerce server-side). Omit for `exists`/`not_exists`.",
									Optional:            true,
								},
							},
						},
					},
				},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current monitor status as last evaluated.",
				Computed:            true,
			},
		},
	}
}

func (r *monitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *monitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.payloadFrom(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.do("POST", "/monitors", payload)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create monitor", err.Error())

		return
	}

	r.applyResponse(ctx, created, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *monitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	monitor, err := r.client.do("GET", "/monitors/"+state.ID.ValueString(), nil)
	if errors.Is(err, errNotFound) {
		resp.State.RemoveResource(ctx)

		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read monitor", err.Error())

		return
	}

	r.applyResponse(ctx, monitor, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *monitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan monitorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.payloadFrom(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.do("PUT", "/monitors/"+plan.ID.ValueString(), payload)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update monitor", err.Error())

		return
	}

	r.applyResponse(ctx, updated, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *monitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.do("DELETE", "/monitors/"+state.ID.ValueString(), nil)
	if err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Failed to delete monitor", err.Error())
	}
}

func (r *monitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// payloadFrom builds the write payload from the plan, sending only the
// attributes the practitioner actually set so server-side defaults stay in
// charge of everything else.
func (r *monitorResource) payloadFrom(ctx context.Context, plan monitorResourceModel, diags *diag.Diagnostics) map[string]any {
	payload := map[string]any{
		"monitor_type": plan.MonitorType.ValueString(),
	}

	if !plan.URL.IsNull() {
		payload["url"] = plan.URL.ValueString()
	}

	if !plan.Port.IsNull() {
		payload["port"] = plan.Port.ValueInt64()
	}
	if !plan.FriendlyName.IsNull() {
		payload["friendly_name"] = plan.FriendlyName.ValueString()
	}
	if !plan.CheckInterval.IsNull() && !plan.CheckInterval.IsUnknown() {
		payload["check_interval"] = plan.CheckInterval.ValueFloat64()
	}
	if !plan.SSLExpiryThreshold.IsNull() && !plan.SSLExpiryThreshold.IsUnknown() {
		payload["ssl_expiry_threshold"] = plan.SSLExpiryThreshold.ValueInt64()
	}
	if !plan.DomainExpiryThreshold.IsNull() && !plan.DomainExpiryThreshold.IsUnknown() {
		payload["domain_expiry_threshold"] = plan.DomainExpiryThreshold.ValueInt64()
	}
	if !plan.RequestTimeout.IsNull() && !plan.RequestTimeout.IsUnknown() {
		payload["request_timeout"] = plan.RequestTimeout.ValueInt64()
	}
	if !plan.FollowRedirects.IsNull() && !plan.FollowRedirects.IsUnknown() {
		payload["follow_redirects"] = plan.FollowRedirects.ValueBool()
	}
	if !plan.ErrorTextDetection.IsNull() && !plan.ErrorTextDetection.IsUnknown() {
		payload["error_text_detection"] = plan.ErrorTextDetection.ValueBool()
	}
	if !plan.HTTPMethod.IsNull() && !plan.HTTPMethod.IsUnknown() {
		payload["http_method"] = plan.HTTPMethod.ValueString()
	}
	if !plan.RequestBody.IsNull() {
		payload["request_body"] = plan.RequestBody.ValueString()
	}
	if !plan.SlowResponseThreshold.IsNull() {
		payload["slow_response_threshold"] = plan.SlowResponseThreshold.ValueInt64()
	}
	if !plan.HeartbeatInterval.IsNull() {
		payload["heartbeat_interval"] = plan.HeartbeatInterval.ValueInt64()
	}
	if !plan.HeartbeatCron.IsNull() {
		payload["heartbeat_cron_expression"] = plan.HeartbeatCron.ValueString()
	}
	if !plan.HeartbeatTimezone.IsNull() {
		payload["heartbeat_timezone"] = plan.HeartbeatTimezone.ValueString()
	}
	if !plan.HeartbeatGrace.IsNull() && !plan.HeartbeatGrace.IsUnknown() {
		payload["heartbeat_grace"] = plan.HeartbeatGrace.ValueInt64()
	}
	if !plan.RequestHeaders.IsNull() && !plan.RequestHeaders.IsUnknown() {
		headers := map[string]string{}
		diags.Append(plan.RequestHeaders.ElementsAs(ctx, &headers, false)...)
		payload["request_headers"] = headers
	}
	if !plan.AcceptedStatusCodes.IsNull() && !plan.AcceptedStatusCodes.IsUnknown() {
		var codes []string
		diags.Append(plan.AcceptedStatusCodes.ElementsAs(ctx, &codes, false)...)
		payload["accepted_status_codes"] = codes
	}

	// group_id and the auth trio are always sent explicitly: the API only
	// updates keys present in the request, so omitting them would make
	// "removed from configuration" silently mean "unchanged".
	if plan.GroupID.IsNull() {
		payload["group_id"] = nil
	} else {
		payload["group_id"] = plan.GroupID.ValueInt64()
	}
	if plan.AuthType.IsNull() {
		payload["auth_type"] = nil
		payload["auth_username"] = nil
		payload["auth_password"] = nil
	} else {
		payload["auth_type"] = plan.AuthType.ValueString()
		payload["auth_username"] = plan.AuthUsername.ValueString()
		payload["auth_password"] = plan.AuthPassword.ValueString()
	}

	payload["keyword_settings"] = keywordSettingsPayload(ctx, plan.KeywordSettings, diags)
	payload["json_assertion_settings"] = jsonAssertionSettingsPayload(ctx, plan.JSONAssertionSettings, diags)

	if !plan.CheckTypes.IsNull() && !plan.CheckTypes.IsUnknown() {
		var checkTypes []string
		diags.Append(plan.CheckTypes.ElementsAs(ctx, &checkTypes, false)...)
		// A settings block that was just removed must take its implied check
		// type with it, or the server rejects the orphaned type for having
		// no settings. The server re-adds the type when settings are present.
		if payload["keyword_settings"] == nil {
			checkTypes = withoutString(checkTypes, "keyword")
		}
		if payload["json_assertion_settings"] == nil {
			checkTypes = withoutString(checkTypes, "json")
		}
		payload["check_types"] = checkTypes
	}
	if !plan.MonitoredRegions.IsNull() && !plan.MonitoredRegions.IsUnknown() {
		var regions []string
		diags.Append(plan.MonitoredRegions.ElementsAs(ctx, &regions, false)...)
		payload["monitored_regions"] = regions
	}

	return payload
}

// applyResponse maps an API monitor object onto the model.
func (r *monitorResource) applyResponse(ctx context.Context, monitor map[string]any, model *monitorResourceModel, diags *diag.Diagnostics) {
	if id, ok := fieldInt(monitor, "id"); ok {
		model.ID = types.StringValue(fmt.Sprintf("%d", id))
	}
	// For push monitors the API's url IS the inbound ping endpoint, which
	// belongs in ping_url; the url attribute stays as configured (null).
	isPush := false
	if t, ok := fieldString(monitor, "monitor_type"); ok {
		isPush = t == "heartbeat" || t == "cron"
	}
	if url, ok := fieldString(monitor, "url"); ok && !isPush {
		model.URL = types.StringValue(url)
	} else if isPush {
		model.URL = types.StringNull()
	}
	if monitorType, ok := fieldString(monitor, "monitor_type"); ok {
		model.MonitorType = types.StringValue(monitorType)
	}
	if name, ok := fieldString(monitor, "friendly_name"); ok {
		model.FriendlyName = types.StringValue(name)
	} else {
		model.FriendlyName = types.StringNull()
	}
	// Optional+Computed attributes must land on a concrete value after
	// apply: the API returns null for unset fields, which maps to explicit
	// null, never unknown.
	if interval, ok := fieldFloat(monitor, "check_interval"); ok {
		model.CheckInterval = types.Float64Value(interval)
	} else {
		model.CheckInterval = types.Float64Null()
	}
	if threshold, ok := fieldInt(monitor, "ssl_expiry_threshold"); ok {
		model.SSLExpiryThreshold = types.Int64Value(threshold)
	} else {
		model.SSLExpiryThreshold = types.Int64Null()
	}
	if threshold, ok := fieldInt(monitor, "domain_expiry_threshold"); ok {
		model.DomainExpiryThreshold = types.Int64Value(threshold)
	} else {
		model.DomainExpiryThreshold = types.Int64Null()
	}
	if timeout, ok := fieldInt(monitor, "request_timeout"); ok {
		model.RequestTimeout = types.Int64Value(timeout)
	} else {
		model.RequestTimeout = types.Int64Null()
	}
	if follow, ok := fieldBool(monitor, "follow_redirects"); ok {
		model.FollowRedirects = types.BoolValue(follow)
	} else {
		model.FollowRedirects = types.BoolNull()
	}
	if detect, ok := fieldBool(monitor, "error_text_detection"); ok {
		model.ErrorTextDetection = types.BoolValue(detect)
	} else {
		model.ErrorTextDetection = types.BoolNull()
	}
	if status, ok := fieldString(monitor, "status"); ok {
		model.Status = types.StringValue(status)
	} else {
		model.Status = types.StringNull()
	}

	// Optional+Computed extras: always mirror the API.
	if method, ok := fieldString(monitor, "http_method"); ok {
		model.HTTPMethod = types.StringValue(method)
	} else {
		model.HTTPMethod = types.StringNull()
	}
	if codes, ok := fieldStringSlice(monitor, "accepted_status_codes"); ok {
		value, valueDiags := types.SetValueFrom(ctx, types.StringType, codes)
		diags.Append(valueDiags...)
		model.AcceptedStatusCodes = value
	} else {
		model.AcceptedStatusCodes = types.SetNull(types.StringType)
	}
	if grace, ok := fieldInt(monitor, "heartbeat_grace"); ok {
		model.HeartbeatGrace = types.Int64Value(grace)
	} else {
		model.HeartbeatGrace = types.Int64Null()
	}
	if pingURL, ok := fieldString(monitor, "ping_url"); ok {
		model.PingURL = types.StringValue(pingURL)
	} else {
		model.PingURL = types.StringNull()
	}

	// Pure-Optional extras: configuration is the authority, so the API value
	// only fills a null/unknown model (fresh imports and creates), never
	// overrides what the practitioner declared.
	if model.RequestBody.IsNull() || model.RequestBody.IsUnknown() {
		if v, ok := fieldString(monitor, "request_body"); ok {
			model.RequestBody = types.StringValue(v)
		} else {
			model.RequestBody = types.StringNull()
		}
	}
	if model.SlowResponseThreshold.IsNull() || model.SlowResponseThreshold.IsUnknown() {
		if v, ok := fieldInt(monitor, "slow_response_threshold"); ok {
			model.SlowResponseThreshold = types.Int64Value(v)
		} else {
			model.SlowResponseThreshold = types.Int64Null()
		}
	}
	if model.HeartbeatInterval.IsNull() || model.HeartbeatInterval.IsUnknown() {
		if v, ok := fieldInt(monitor, "heartbeat_interval"); ok {
			model.HeartbeatInterval = types.Int64Value(v)
		} else {
			model.HeartbeatInterval = types.Int64Null()
		}
	}
	if model.HeartbeatCron.IsNull() || model.HeartbeatCron.IsUnknown() {
		if v, ok := fieldString(monitor, "heartbeat_cron_expression"); ok {
			model.HeartbeatCron = types.StringValue(v)
		} else {
			model.HeartbeatCron = types.StringNull()
		}
	}
	if v, ok := fieldString(monitor, "heartbeat_timezone"); ok {
		model.HeartbeatTimezone = types.StringValue(v)
	} else {
		model.HeartbeatTimezone = types.StringNull()
	}
	if model.RequestHeaders.IsNull() || model.RequestHeaders.IsUnknown() {
		if headers, ok := fieldStringMap(monitor, "request_headers"); ok {
			value, valueDiags := types.MapValueFrom(ctx, types.StringType, headers)
			diags.Append(valueDiags...)
			model.RequestHeaders = value
		} else {
			model.RequestHeaders = types.MapNull(types.StringType)
		}
	}

	// group_id, auth_type, and auth_username mirror the API for drift
	// detection (the API returns auth_username to write-capable tokens);
	// auth_password is write-only and stays exactly as configured.
	if groupID, ok := fieldInt(monitor, "group_id"); ok {
		model.GroupID = types.Int64Value(groupID)
	} else {
		model.GroupID = types.Int64Null()
	}
	if authType, ok := fieldString(monitor, "auth_type"); ok {
		model.AuthType = types.StringValue(authType)
	} else {
		model.AuthType = types.StringNull()
	}
	if authUsername, ok := fieldString(monitor, "auth_username"); ok {
		model.AuthUsername = types.StringValue(authUsername)
	} else {
		model.AuthUsername = types.StringNull()
	}
	if model.AuthPassword.IsUnknown() {
		model.AuthPassword = types.StringNull()
	}

	model.KeywordSettings = keywordSettingsFromAPI(ctx, monitor, diags)
	model.JSONAssertionSettings = jsonAssertionSettingsFromAPI(ctx, monitor, diags)

	if regions, ok := fieldStringSlice(monitor, "monitored_regions"); ok {
		value, valueDiags := types.SetValueFrom(ctx, types.StringType, regions)
		diags.Append(valueDiags...)
		model.MonitoredRegions = value
	} else {
		model.MonitoredRegions = types.SetNull(types.StringType)
	}
	if port, hasPort := fieldInt(monitor, "port"); hasPort {
		model.Port = types.Int64Value(port)
	} else if model.Port.IsUnknown() {
		model.Port = types.Int64Null()
	}

	// Optional+Computed: always mirror the API (imports start from a null
	// model and must still pick these up); Set comparison ignores order.
	if checkTypes, ok := fieldStringSlice(monitor, "check_types"); ok {
		value, valueDiags := types.SetValueFrom(ctx, types.StringType, checkTypes)
		diags.Append(valueDiags...)
		model.CheckTypes = value
	} else {
		model.CheckTypes = types.SetNull(types.StringType)
	}
}

/* ------------------------------------------------------------------
| keyword_settings / json_assertion_settings conversion
|
| Plan objects become the API's JSON shape (omitting unset optionals so
| the server stores only what the practitioner declared), and API
| responses become state objects, so both blocks drift-detect.
|------------------------------------------------------------------ */

func withoutString(values []string, remove string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != remove {
			out = append(out, v)
		}
	}

	return out
}

// keywordSettingsPayload returns nil (JSON null) when the block is absent.
func keywordSettingsPayload(ctx context.Context, object types.Object, diags *diag.Diagnostics) any {
	if object.IsNull() || object.IsUnknown() {
		return nil
	}

	var settings keywordSettingsModel
	diags.Append(object.As(ctx, &settings, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}

	keywords := make([]map[string]any, 0, len(settings.Keywords))
	for _, keyword := range settings.Keywords {
		entry := map[string]any{
			"phrase": keyword.Phrase.ValueString(),
			"mode":   keyword.Mode.ValueString(),
		}
		if !keyword.CaseSensitive.IsNull() {
			entry["case_sensitive"] = keyword.CaseSensitive.ValueBool()
		}
		keywords = append(keywords, entry)
	}

	payload := map[string]any{"keywords": keywords}
	if !settings.SearchTarget.IsNull() {
		payload["search_target"] = settings.SearchTarget.ValueString()
	}

	return payload
}

func jsonAssertionSettingsPayload(ctx context.Context, object types.Object, diags *diag.Diagnostics) any {
	if object.IsNull() || object.IsUnknown() {
		return nil
	}

	var settings jsonAssertionSettingsModel
	diags.Append(object.As(ctx, &settings, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}

	assertions := make([]map[string]any, 0, len(settings.Assertions))
	for _, assertion := range settings.Assertions {
		entry := map[string]any{
			"path":     assertion.Path.ValueString(),
			"operator": assertion.Operator.ValueString(),
		}
		if !assertion.Value.IsNull() {
			entry["value"] = assertion.Value.ValueString()
		}
		assertions = append(assertions, entry)
	}

	return map[string]any{"assertions": assertions}
}

func keywordSettingsFromAPI(ctx context.Context, monitor map[string]any, diags *diag.Diagnostics) types.Object {
	raw, ok := monitor["keyword_settings"].(map[string]any)
	if !ok {
		return types.ObjectNull(keywordSettingsAttrTypes)
	}

	settings := keywordSettingsModel{
		SearchTarget: types.StringNull(),
		Keywords:     []keywordEntryModel{},
	}
	if target, ok := fieldString(raw, "search_target"); ok {
		settings.SearchTarget = types.StringValue(target)
	}

	if entries, ok := raw["keywords"].([]any); ok {
		for _, item := range entries {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}

			keyword := keywordEntryModel{
				Phrase:        types.StringNull(),
				Mode:          types.StringNull(),
				CaseSensitive: types.BoolNull(),
			}
			if phrase, ok := fieldString(entry, "phrase"); ok {
				keyword.Phrase = types.StringValue(phrase)
			}
			if mode, ok := fieldString(entry, "mode"); ok {
				keyword.Mode = types.StringValue(mode)
			}
			if sensitive, ok := fieldBool(entry, "case_sensitive"); ok {
				keyword.CaseSensitive = types.BoolValue(sensitive)
			}
			settings.Keywords = append(settings.Keywords, keyword)
		}
	}

	object, objectDiags := types.ObjectValueFrom(ctx, keywordSettingsAttrTypes, settings)
	diags.Append(objectDiags...)

	return object
}

func jsonAssertionSettingsFromAPI(ctx context.Context, monitor map[string]any, diags *diag.Diagnostics) types.Object {
	raw, ok := monitor["json_assertion_settings"].(map[string]any)
	if !ok {
		return types.ObjectNull(jsonAssertionSettingsAttrTypes)
	}

	settings := jsonAssertionSettingsModel{Assertions: []jsonAssertionEntryModel{}}

	if entries, ok := raw["assertions"].([]any); ok {
		for _, item := range entries {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}

			assertion := jsonAssertionEntryModel{
				Path:     types.StringNull(),
				Operator: types.StringNull(),
				Value:    types.StringNull(),
			}
			if path, ok := fieldString(entry, "path"); ok {
				assertion.Path = types.StringValue(path)
			}
			if operator, ok := fieldString(entry, "operator"); ok {
				assertion.Operator = types.StringValue(operator)
			}
			if value, ok := fieldString(entry, "value"); ok {
				assertion.Value = types.StringValue(value)
			}
			settings.Assertions = append(settings.Assertions, assertion)
		}
	}

	object, objectDiags := types.ObjectValueFrom(ctx, jsonAssertionSettingsAttrTypes, settings)
	diags.Append(objectDiags...)

	return object
}
