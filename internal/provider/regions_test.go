package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSameRegionsAcrossNameGenerations(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{[]string{"ash", "sin"}, []string{"us-east", "ap-southeast"}, true},
		{[]string{"nyc", "ash", "pdx"}, []string{"us-west", "us-east"}, true},
		{[]string{"ash"}, []string{"us-east", "us-west"}, false},
		{[]string{"nbg"}, []string{"ap-southeast"}, false},
		{[]string{"mars"}, []string{"mars"}, true},
	}
	for _, c := range cases {
		if got := sameRegions(c.a, c.b); got != c.want {
			t.Errorf("sameRegions(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// A configuration written with airport codes must not drift or fail apply
// when the API returns location names for the same regions.
func TestDeclaredOldRegionNamesSurviveNewNamesFromTheAPI(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	r := &monitorResource{}

	declared, _ := types.SetValueFrom(ctx, types.StringType, []string{"ash", "sin"})
	model := monitorResourceModel{MonitoredRegions: declared}
	api := map[string]any{"id": "1", "monitor_type": "http", "monitored_regions": []any{"us-east", "ap-southeast"}}

	r.applyResponse(ctx, api, &model, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !model.MonitoredRegions.Equal(declared) {
		t.Fatalf("monitored_regions = %v, want the declared %v", model.MonitoredRegions, declared)
	}
}

// A real change of regions still comes through from the API.
func TestDifferentRegionsFromTheAPIReplaceState(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	r := &monitorResource{}

	prior, _ := types.SetValueFrom(ctx, types.StringType, []string{"ash"})
	model := monitorResourceModel{MonitoredRegions: prior}
	api := map[string]any{"id": "1", "monitor_type": "http", "monitored_regions": []any{"us-east", "eu-central"}}

	r.applyResponse(ctx, api, &model, &diags)
	want, _ := types.SetValueFrom(ctx, types.StringType, []string{"us-east", "eu-central"})
	if !model.MonitoredRegions.Equal(want) {
		t.Fatalf("monitored_regions = %v, want %v", model.MonitoredRegions, want)
	}
}
