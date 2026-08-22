package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

/*
sentinel_group maps to /api/v1/groups: the two-level tree the dashboard
manages. Monitors join a group through the sentinel_monitor group_id
attribute. Deleting a group server-side ungroups its monitors and promotes
its subgroups; it never deletes monitors, so destroy is safe.

monitors_count and sort_order are deliberately NOT modeled: both change
outside Terraform (monitors join/leave, the dashboard reorders) and would
show up as perpetual refresh churn on state the practitioner never set.
*/

func NewGroupResource() resource.Resource {
	return &groupResource{}
}

type groupResource struct {
	client *apiClient
}

type groupResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	ParentID    types.Int64  `tfsdk:"parent_id"`
}

func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A monitor group: the organizational unit the dashboard's grouped view is built from. Assign monitors with the `group_id` attribute on `sentinel_monitor`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group name, unique within the team, max 60 characters.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description, max 255 characters.",
				Optional:            true,
			},
			"parent_id": schema.Int64Attribute{
				MarkdownDescription: "Parent group id for a subgroup. Nesting is one level deep: the parent must be a top-level group, and a group with subgroups cannot itself be nested.",
				Optional:            true,
			},
		},
	}
}

func (r *groupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.do("POST", "/groups", r.payloadFrom(plan))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create group", err.Error())

		return
	}

	r.applyResponse(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.client.do("GET", "/groups/"+state.ID.ValueString(), nil)
	if errors.Is(err, errNotFound) {
		resp.State.RemoveResource(ctx)

		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read group", err.Error())

		return
	}

	r.applyResponse(group, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.do("PUT", "/groups/"+plan.ID.ValueString(), r.payloadFrom(plan))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update group", err.Error())

		return
	}

	r.applyResponse(updated, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.do("DELETE", "/groups/"+state.ID.ValueString(), nil)
	if err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Failed to delete group", err.Error())
	}
}

func (r *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *groupResource) payloadFrom(plan groupResourceModel) map[string]any {
	payload := map[string]any{
		"name": plan.Name.ValueString(),
	}

	if !plan.Description.IsNull() {
		payload["description"] = plan.Description.ValueString()
	}
	// parent_id is always sent: omitting it on update would silently promote
	// a subgroup to the top level, so null must mean null explicitly.
	if plan.ParentID.IsNull() {
		payload["parent_id"] = nil
	} else {
		payload["parent_id"] = plan.ParentID.ValueInt64()
	}

	return payload
}

func (r *groupResource) applyResponse(group map[string]any, model *groupResourceModel) {
	if id, ok := fieldInt(group, "id"); ok {
		model.ID = types.StringValue(fmt.Sprintf("%d", id))
	}
	if name, ok := fieldString(group, "name"); ok {
		model.Name = types.StringValue(name)
	}
	if description, ok := fieldString(group, "description"); ok {
		model.Description = types.StringValue(description)
	} else {
		model.Description = types.StringNull()
	}
	if parentID, ok := fieldInt(group, "parent_id"); ok {
		model.ParentID = types.Int64Value(parentID)
	} else {
		model.ParentID = types.Int64Null()
	}
}
