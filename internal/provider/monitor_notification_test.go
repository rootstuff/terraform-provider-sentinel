package provider

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The routing block must be omitted from requests when undeclared (the API
// leaves routing alone only when the key is absent) and must mirror back
// exactly what was declared, or Terraform reports an inconsistent result
// after apply.

func declaredNotificationSettings(t *testing.T, ctx context.Context) types.Object {
	t.Helper()

	channels, diags := types.MapValueFrom(ctx, notificationChannelsType.ElemType, map[string][]string{
		"email": {"critical", "warning"},
		"slack": {"critical"},
	})
	if diags.HasError() {
		t.Fatalf("channels: %v", diags)
	}
	quiet, diags := types.ObjectValue(notificationQuietHoursAttrTypes, map[string]attr.Value{
		"enabled":         types.BoolValue(true),
		"start":           types.StringValue("22:00"),
		"end":             types.StringValue("07:00"),
		"timezone":        types.StringNull(),
		"bypass_critical": types.BoolValue(true),
	})
	if diags.HasError() {
		t.Fatalf("quiet hours: %v", diags)
	}
	object, diags := types.ObjectValue(notificationSettingsAttrTypes, map[string]attr.Value{
		"enabled":     types.BoolValue(true),
		"channels":    channels,
		"quiet_hours": quiet,
	})
	if diags.HasError() {
		t.Fatalf("settings: %v", diags)
	}

	return object
}

func TestNotificationSettingsOmittedWhenUndeclared(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	if payload := notificationSettingsPayload(ctx, types.ObjectNull(notificationSettingsAttrTypes), &diags); payload != nil {
		t.Fatalf("expected no payload, got %v", payload)
	}

	api := map[string]any{"notification_settings": map[string]any{"enabled": true, "channels": map[string]any{"sms": []any{"critical"}}}}
	mirrored := notificationSettingsFromAPI(ctx, api, types.ObjectNull(notificationSettingsAttrTypes), &diags)
	if !mirrored.IsNull() {
		t.Fatalf("undeclared block must stay null whatever the API holds, got %v", mirrored)
	}
}

func TestNotificationSettingsPayloadSendsOnlyDeclaredBlocks(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	payload := notificationSettingsPayload(ctx, declaredNotificationSettings(t, ctx), &diags)
	if diags.HasError() {
		t.Fatalf("payload: %v", diags)
	}

	want := map[string]any{
		"enabled":  true,
		"channels": map[string][]string{"email": {"critical", "warning"}, "slack": {"critical"}},
		"quiet_hours": map[string]any{
			"enabled": true, "start": "22:00", "end": "07:00", "bypass_critical": true,
		},
	}
	if !reflect.DeepEqual(payload, want) {
		t.Fatalf("payload mismatch\n got %#v\nwant %#v", payload, want)
	}
}

func TestNotificationSettingsRoundTripMatchesPlan(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	declared := declaredNotificationSettings(t, ctx)
	// What the API echoes after storing that exact payload (JSON-decoded shapes).
	api := map[string]any{"notification_settings": map[string]any{
		"enabled":  true,
		"channels": map[string]any{"email": []any{"critical", "warning"}, "slack": []any{"critical"}},
		"quiet_hours": map[string]any{
			"enabled": true, "start": "22:00", "end": "07:00", "bypass_critical": true,
		},
		"custom_recipients": map[string]any{"emails": []any{"ops@example.com"}},
	}}

	mirrored := notificationSettingsFromAPI(ctx, api, declared, &diags)
	if diags.HasError() {
		t.Fatalf("mirror: %v", diags)
	}
	if !mirrored.Equal(declared) {
		t.Fatalf("state must equal plan after apply\n got %v\nwant %v", mirrored, declared)
	}
}

func TestNotificationSettingsEmptiedChannelsMirrorAsEmptyMap(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	channels, _ := types.MapValueFrom(ctx, notificationChannelsType.ElemType, map[string][]string{})
	declared, _ := types.ObjectValue(notificationSettingsAttrTypes, map[string]attr.Value{
		"enabled":     types.BoolNull(),
		"channels":    channels,
		"quiet_hours": types.ObjectNull(notificationQuietHoursAttrTypes),
	})
	// PHP serialises an emptied associative array as a JSON list.
	api := map[string]any{"notification_settings": map[string]any{"enabled": true, "channels": []any{}}}

	mirrored := notificationSettingsFromAPI(ctx, api, declared, &diags)
	if diags.HasError() {
		t.Fatalf("mirror: %v", diags)
	}
	if !mirrored.Equal(declared) {
		t.Fatalf("emptied channels should mirror as an empty map and enabled stay null\n got %v\nwant %v", mirrored, declared)
	}
}

func TestNotificationSettingsInAppAliasTranslatesBothWays(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	channels, _ := types.MapValueFrom(ctx, notificationChannelsType.ElemType, map[string][]string{"in_app": {"critical", "info"}, "email": {"critical"}})
	declared, _ := types.ObjectValue(notificationSettingsAttrTypes, map[string]attr.Value{
		"enabled":     types.BoolNull(),
		"channels":    channels,
		"quiet_hours": types.ObjectNull(notificationQuietHoursAttrTypes),
	})

	payload := notificationSettingsPayload(ctx, declared, &diags)
	if diags.HasError() {
		t.Fatalf("payload: %v", diags)
	}
	wantChannels := map[string][]string{"database": {"critical", "info"}, "email": {"critical"}}
	if !reflect.DeepEqual(payload["channels"], wantChannels) {
		t.Fatalf("alias must be sent under the stored name\n got %#v\nwant %#v", payload["channels"], wantChannels)
	}

	// The API answers with the stored name; state must come back as declared.
	api := map[string]any{"notification_settings": map[string]any{
		"channels": map[string]any{"database": []any{"critical", "info"}, "email": []any{"critical"}},
	}}
	mirrored := notificationSettingsFromAPI(ctx, api, declared, &diags)
	if diags.HasError() {
		t.Fatalf("mirror: %v", diags)
	}
	if !mirrored.Equal(declared) {
		t.Fatalf("state must equal plan when the alias is used\n got %v\nwant %v", mirrored, declared)
	}
}
