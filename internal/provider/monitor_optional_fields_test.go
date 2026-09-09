package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An HTTP monitor whose row carries heartbeat_interval = 300 (the dashboard
// form writes it for every type). The plan left the attribute out, so state
// must stay null or Terraform reports an inconsistent result after apply.
func TestUndeclaredOptionalFieldsStayNullWhateverTheAPIReturns(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	r := &monitorResource{}

	api := map[string]any{
		"id":                        json.Number("224"),
		"monitor_type":              "http",
		"url":                       "https://api.rootly.com/v1/heartbeats/x/ping",
		"heartbeat_interval":        json.Number("300"),
		"heartbeat_cron_expression": "*/5 * * * *",
		"slow_response_threshold":   json.Number("2000"),
		"request_body":              "{}",
		"request_headers":           map[string]any{"X-Test": "1"},
	}

	model := monitorResourceModel{
		HeartbeatInterval:     types.Int64Null(),
		HeartbeatCron:         types.StringNull(),
		SlowResponseThreshold: types.Int64Null(),
		RequestBody:           types.StringNull(),
		RequestHeaders:        types.MapNull(types.StringType),
	}
	r.applyResponse(ctx, api, &model, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if !model.HeartbeatInterval.IsNull() {
		t.Fatalf("heartbeat_interval must stay null when undeclared, got %v", model.HeartbeatInterval)
	}
	if !model.HeartbeatCron.IsNull() || !model.SlowResponseThreshold.IsNull() || !model.RequestBody.IsNull() || !model.RequestHeaders.IsNull() {
		t.Fatalf("undeclared optional attributes must stay null: cron=%v slow=%v body=%v headers=%v",
			model.HeartbeatCron, model.SlowResponseThreshold, model.RequestBody, model.RequestHeaders)
	}
}

// A declared value is never overwritten by the API, and an unknown value
// (one derived from another resource) is filled from it.
func TestDeclaredOptionalFieldsKeepTheirValueAndUnknownOnesAreFilled(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	r := &monitorResource{}

	api := map[string]any{
		"id":                      json.Number("1"),
		"monitor_type":            "heartbeat",
		"heartbeat_interval":      json.Number("300"),
		"slow_response_threshold": json.Number("2000"),
	}

	model := monitorResourceModel{
		HeartbeatInterval:     types.Int64Value(600),
		SlowResponseThreshold: types.Int64Unknown(),
		RequestBody:           types.StringNull(),
		RequestHeaders:        types.MapNull(types.StringType),
	}
	r.applyResponse(ctx, api, &model, &diags)

	if model.HeartbeatInterval.ValueInt64() != 600 {
		t.Fatalf("declared heartbeat_interval must win over the API, got %v", model.HeartbeatInterval)
	}
	if model.SlowResponseThreshold.ValueInt64() != 2000 {
		t.Fatalf("unknown slow_response_threshold must be filled from the API, got %v", model.SlowResponseThreshold)
	}
}
