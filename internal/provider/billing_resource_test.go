package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBillingPayloadSendsEveryFieldSoOmittedMeansNone(t *testing.T) {
	ctx := context.Background()
	recipients, _ := types.ListValueFrom(ctx, types.StringType, []string{"accounting@example.com"})
	plan := billingResourceModel{
		Recipients: recipients,
		Company:    types.StringValue("Example Company LLC"),
		Line1:      types.StringValue("1 Main St"),
		Country:    types.StringValue("US"),
		TaxIDType:  types.StringValue("us_ein"),
		TaxID:      types.StringValue("12-3456789"),
		// Line2, City, State, PostalCode left null on purpose.
	}

	payload := (&billingResource{}).payloadFrom(ctx, plan)

	got := payload["billing_recipients"].([]string)
	if len(got) != 1 || got[0] != "accounting@example.com" {
		t.Fatalf("recipients = %v", got)
	}
	info := payload["billing_information"].(map[string]any)
	address := info["address"].(map[string]any)
	if info["company"] != "Example Company LLC" || address["line1"] != "1 Main St" || address["country"] != "US" {
		t.Fatalf("information = %v", info)
	}
	if address["city"] != "" || address["line2"] != "" {
		t.Fatalf("omitted address fields must be sent as empty strings so the API clears them: %v", address)
	}
	if info["tax_id_type"] != "us_ein" || info["tax_id"] != "12-3456789" {
		t.Fatalf("tax id = %v / %v", info["tax_id_type"], info["tax_id"])
	}
}

func TestBillingApplyResponseReadsTheDataEnvelopeAndNullsEmptyFields(t *testing.T) {
	ctx := context.Background()
	envelope := map[string]any{
		"data": map[string]any{
			"plan":               "business",
			"billing_recipients": []any{"accounting@example.com", "ap@example.com"},
			"billing_information": map[string]any{
				"company":     "Example Company LLC",
				"address":     map[string]any{"line1": "1 Main St", "country": "US"},
				"tax_id_type": nil,
				"tax_id":      nil,
			},
		},
	}
	var model billingResourceModel

	(&billingResource{}).applyResponse(ctx, envelope, &model)

	if model.ID.ValueString() != "billing" {
		t.Fatalf("id = %q", model.ID.ValueString())
	}
	var recipients []string
	_ = model.Recipients.ElementsAs(ctx, &recipients, false)
	if len(recipients) != 2 || recipients[1] != "ap@example.com" {
		t.Fatalf("recipients = %v", recipients)
	}
	if model.Company.ValueString() != "Example Company LLC" || model.Line1.ValueString() != "1 Main St" || model.Country.ValueString() != "US" {
		t.Fatalf("information not applied: %+v", model)
	}
	if !model.City.IsNull() || !model.TaxIDType.IsNull() || !model.TaxID.IsNull() {
		t.Fatalf("fields the API omitted or sent empty must be null, got city=%v tax_id_type=%v tax_id=%v", model.City, model.TaxIDType, model.TaxID)
	}
}

func TestBillingApplyResponseNullsRecipientsWhenEmpty(t *testing.T) {
	ctx := context.Background()
	var model billingResourceModel

	(&billingResource{}).applyResponse(ctx, map[string]any{"data": map[string]any{"billing_recipients": []any{}, "billing_information": map[string]any{}}}, &model)

	if !model.Recipients.IsNull() {
		t.Fatalf("empty recipients must be null so an unset attribute never diffs, got %v", model.Recipients)
	}
}
