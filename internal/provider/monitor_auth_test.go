package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func authPayload(t *testing.T, authType, username, password types.String) (map[string]any, diag.Diagnostics) {
	t.Helper()
	var diags diag.Diagnostics
	r := &monitorResource{}
	plan := monitorResourceModel{
		MonitorType:  types.StringValue("http"),
		URL:          types.StringValue("https://api.example.com/health"),
		AuthType:     authType,
		AuthUsername: username,
		AuthPassword: password,
	}

	return r.payloadFrom(context.Background(), plan, &diags), diags
}

// Bearer auth carries the token in auth_password and no username: the API
// stores none for bearer, so sending one would only come back as drift.
func TestBearerAuthSendsTheTokenAsThePasswordAndNoUsername(t *testing.T) {
	payload, diags := authPayload(t, types.StringValue("bearer"), types.StringNull(), types.StringValue("SG.tok"))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if payload["auth_type"] != "bearer" || payload["auth_password"] != "SG.tok" || payload["auth_username"] != nil {
		t.Fatalf("bearer payload wrong: type=%v user=%v pass=%v", payload["auth_type"], payload["auth_username"], payload["auth_password"])
	}
}

func TestBasicAuthSendsBothCredentials(t *testing.T) {
	payload, diags := authPayload(t, types.StringValue("basic"), types.StringValue("ops"), types.StringValue("pw"))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if payload["auth_type"] != "basic" || payload["auth_username"] != "ops" || payload["auth_password"] != "pw" {
		t.Fatalf("basic payload wrong: %v", payload)
	}
}

func TestNoAuthClearsAllThreeFields(t *testing.T) {
	payload, diags := authPayload(t, types.StringNull(), types.StringNull(), types.StringNull())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	for _, key := range []string{"auth_type", "auth_username", "auth_password"} {
		if value, present := payload[key]; !present || value != nil {
			t.Fatalf("%s must be sent as null, got %v (present=%v)", key, value, present)
		}
	}
}

func TestAuthMisconfigurationsAreReportedBeforeTheRequest(t *testing.T) {
	cases := map[string]struct{ authType, username, password types.String }{
		"basic without a username":   {types.StringValue("basic"), types.StringNull(), types.StringValue("pw")},
		"bearer with a username":     {types.StringValue("bearer"), types.StringValue("ops"), types.StringValue("tok")},
		"digest was never honoured":  {types.StringValue("digest"), types.StringValue("ops"), types.StringValue("pw")},
		"auth_type without a secret": {types.StringValue("bearer"), types.StringNull(), types.StringNull()},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, diags := authPayload(t, c.authType, c.username, c.password)
			if !diags.HasError() {
				t.Fatalf("expected a diagnostic for %s", name)
			}
		})
	}
}
