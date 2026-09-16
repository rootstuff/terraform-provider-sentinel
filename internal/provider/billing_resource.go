package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

/*
sentinel_billing maps to /api/v1/billing: the token owner's billing
recipients (every paid invoice is emailed to them as a PDF) and billing
information (the company, address and tax ID printed on invoices, pushed to
the Stripe customer). There is exactly one per account, so the resource is
a singleton: its id is always "billing", and destroying it clears both
blocks rather than deleting anything server-side.

Billing belongs to the plan owner, not to a team. The token used by the
provider must belong to the account that pays; a member's token would
manage that member's own (usually free) billing instead.
*/

func NewBillingResource() resource.Resource {
	return &billingResource{}
}

type billingResource struct {
	client *apiClient
}

type billingResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Recipients types.List   `tfsdk:"recipients"`
	Company    types.String `tfsdk:"company"`
	Line1      types.String `tfsdk:"address_line1"`
	Line2      types.String `tfsdk:"address_line2"`
	City       types.String `tfsdk:"city"`
	State      types.String `tfsdk:"state"`
	PostalCode types.String `tfsdk:"postal_code"`
	Country    types.String `tfsdk:"country"`
	TaxIDType  types.String `tfsdk:"tax_id_type"`
	TaxID      types.String `tfsdk:"tax_id"`
}

func (r *billingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing"
}

func (r *billingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The account's billing recipients and the company details printed on invoices. One per account (the token owner's); destroying it clears both.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"recipients": schema.ListAttribute{
				MarkdownDescription: "Up to five email addresses that receive every paid invoice as a PDF. Stored lower-cased. Omit or leave empty for none.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"company": schema.StringAttribute{
				MarkdownDescription: "Company or legal name invoices are made out to. Omit to invoice the account holder by name. Does not change the account's own name.",
				Optional:            true,
			},
			"address_line1": schema.StringAttribute{Optional: true, MarkdownDescription: "Street address."},
			"address_line2": schema.StringAttribute{Optional: true, MarkdownDescription: "Suite, floor or building."},
			"city":          schema.StringAttribute{Optional: true},
			"state":         schema.StringAttribute{Optional: true, MarkdownDescription: "State or region."},
			"postal_code":   schema.StringAttribute{Optional: true},
			"country": schema.StringAttribute{
				MarkdownDescription: "Two-letter country code, for example `US` or `DE`.",
				Optional:            true,
			},
			"tax_id_type": schema.StringAttribute{
				MarkdownDescription: "Kind of tax ID, one of the keys `GET /api/v1/billing` lists under `tax_id_types`, for example `us_ein`, `eu_vat`, `gb_vat`. Required when `tax_id` is set.",
				Optional:            true,
			},
			"tax_id": schema.StringAttribute{
				MarkdownDescription: "The tax or VAT number.",
				Optional:            true,
			},
		},
	}
}

func (r *billingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *billingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan billingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.put(ctx, &plan, resp.Diagnostics.AddError, resp.Diagnostics.AddWarning)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *billingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state billingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	envelope, err := r.client.do("GET", "/billing", nil)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read billing", err.Error())

		return
	}

	r.applyResponse(ctx, envelope, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *billingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan billingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.put(ctx, &plan, resp.Diagnostics.AddError, resp.Diagnostics.AddWarning)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete clears both blocks: there is nothing to delete server-side, and
// "no longer managed by Terraform" should not leave stale recipients or a
// company name behind that nobody declared.
func (r *billingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	_, err := r.client.do("PUT", "/billing", map[string]any{
		"billing_recipients":  []string{},
		"billing_information": map[string]any{},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to clear billing", err.Error())
	}
}

func (r *billingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Any id imports the one billing record; it is always stored as "billing".
	resource.ImportStatePassthroughID(ctx, path.Root("id"), resource.ImportStateRequest{ID: "billing"}, resp)
}

func (r *billingResource) put(ctx context.Context, plan *billingResourceModel, addError func(string, string), addWarning func(string, string)) {
	envelope, err := r.client.do("PUT", "/billing", r.payloadFrom(ctx, *plan))
	if err != nil {
		addError("Failed to save billing", err.Error())

		return
	}

	if warning, ok := fieldString(envelope, "warning"); ok && warning != "" {
		addWarning("Stripe rejected the billing information", warning)
	}

	r.applyResponse(ctx, envelope, plan)
}

func (r *billingResource) payloadFrom(ctx context.Context, plan billingResourceModel) map[string]any {
	recipients := []string{}
	if !plan.Recipients.IsNull() && !plan.Recipients.IsUnknown() {
		_ = plan.Recipients.ElementsAs(ctx, &recipients, false)
	}

	str := func(v types.String) string {
		if v.IsNull() || v.IsUnknown() {
			return ""
		}

		return v.ValueString()
	}

	// Every field is always sent: the resource declares the whole record,
	// so an attribute left out of the config means "none", not "keep".
	return map[string]any{
		"billing_recipients": recipients,
		"billing_information": map[string]any{
			"company": str(plan.Company),
			"address": map[string]any{
				"line1":       str(plan.Line1),
				"line2":       str(plan.Line2),
				"city":        str(plan.City),
				"state":       str(plan.State),
				"postal_code": str(plan.PostalCode),
				"country":     str(plan.Country),
			},
			"tax_id_type": str(plan.TaxIDType),
			"tax_id":      str(plan.TaxID),
		},
	}
}

func (r *billingResource) applyResponse(ctx context.Context, envelope map[string]any, model *billingResourceModel) {
	model.ID = types.StringValue("billing")

	data, _ := envelope["data"].(map[string]any)
	if data == nil {
		return
	}

	recipients, _ := fieldStringSlice(data, "billing_recipients")
	if len(recipients) == 0 {
		model.Recipients = types.ListNull(types.StringType)
	} else {
		list, _ := types.ListValueFrom(ctx, types.StringType, recipients)
		model.Recipients = list
	}

	info, _ := data["billing_information"].(map[string]any)
	if info == nil {
		info = map[string]any{}
	}
	address, _ := info["address"].(map[string]any)
	if address == nil {
		address = map[string]any{}
	}

	model.Company = optionalString(info, "company")
	model.Line1 = optionalString(address, "line1")
	model.Line2 = optionalString(address, "line2")
	model.City = optionalString(address, "city")
	model.State = optionalString(address, "state")
	model.PostalCode = optionalString(address, "postal_code")
	model.Country = optionalString(address, "country")
	model.TaxIDType = optionalString(info, "tax_id_type")
	model.TaxID = optionalString(info, "tax_id")
}

// optionalString reads a string field as a Terraform value, null when the
// API omits it or sends an empty string, so an attribute the practitioner
// never set never shows up as a diff.
func optionalString(m map[string]any, key string) types.String {
	if value, ok := fieldString(m, key); ok && value != "" {
		return types.StringValue(value)
	}

	return types.StringNull()
}
