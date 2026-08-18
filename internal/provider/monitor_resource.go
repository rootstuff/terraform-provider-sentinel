package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

	if !plan.CheckTypes.IsNull() && !plan.CheckTypes.IsUnknown() {
		var checkTypes []string
		diags.Append(plan.CheckTypes.ElementsAs(ctx, &checkTypes, false)...)
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
