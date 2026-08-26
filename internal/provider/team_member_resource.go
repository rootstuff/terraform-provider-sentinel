package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

/*
sentinel_team_member models an access GRANT, not a user account. Sentinel
membership is invitation-based: creating this resource sends an invitation
email, and the person becomes a member when they accept. Both states satisfy
the resource ("access is granted"), so the identity is the EMAIL, which is
stable across the invitation-to-member transition; the numeric invitation and
user ids are implementation details resolved on every operation.

Consequences of that model:
  - `status` reports where the grant currently stands (invited or active) and
    flips out of band when the invitation is accepted; it is Computed and
    never produces a diff.
  - Changing `role` updates in place on either side of the transition (the
    pending invitation is re-roled without a second email).
  - Destroy revokes whichever exists: the pending invitation or the
    membership. The person's Sentinel account is never deleted.
  - The team owner is not manageable here: the API refuses to re-role or
    remove the owner, and this resource surfaces that refusal.
*/

func NewTeamMemberResource() resource.Resource {
	return &teamMemberResource{}
}

type teamMemberResource struct {
	client *apiClient
}

type teamMemberResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Email  types.String `tfsdk:"email"`
	Role   types.String `tfsdk:"role"`
	Status types.String `tfsdk:"status"`
	UserID types.Int64  `tfsdk:"user_id"`
}

// teamGrant is the resolved server-side state for an email: a pending
// invitation, an active membership, or neither.
type teamGrant struct {
	kind   string // "invitation" | "member" | ""
	id     int64  // invitation id or user id, depending on kind
	role   string
	status string // "invited", or the member status (active/disabled)
}

func (r *teamMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_member"
}

func (r *teamMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An access grant to the team, keyed by email. Creating it sends a Sentinel invitation email; the grant stays managed through the invitation's acceptance (`status` moves from `invited` to `active`). Destroying it cancels the pending invitation or removes the member, whichever exists. The team owner cannot be managed with this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Same as `email`: the grant's stable identity across the invitation-to-member transition.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "Email address the grant is for. Changing it replaces the resource (revokes the old grant, invites the new address).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				MarkdownDescription: "Team role: `admin`, `editor`, or `viewer`. Updates in place, whether the grant is still a pending invitation or an accepted membership.",
				Required:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "`invited` while the invitation is pending, then `active` (or `disabled` if the account is suspended) once accepted.",
				Computed:            true,
			},
			"user_id": schema.Int64Attribute{
				MarkdownDescription: "The member's user id once the invitation is accepted; null while pending.",
				Computed:            true,
			},
		},
	}
}

func (r *teamMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *teamMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan teamMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.do("POST", "/users/invitations", map[string]any{
		"email": plan.Email.ValueString(),
		"role":  plan.Role.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to invite team member", err.Error())

		return
	}

	grant, err := r.resolveGrant(plan.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read back the invitation", err.Error())

		return
	}

	r.applyGrant(grant, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *teamMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Import sets only id; the id IS the email.
	email := state.Email.ValueString()
	if email == "" {
		email = state.ID.ValueString()
		state.Email = types.StringValue(email)
	}

	grant, err := r.resolveGrant(email)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read team member", err.Error())

		return
	}
	if grant.kind == "" {
		// Neither invited nor a member: the grant was revoked out of band.
		resp.State.RemoveResource(ctx)

		return
	}

	r.applyGrant(grant, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *teamMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan teamMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	grant, err := r.resolveGrant(plan.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve team member", err.Error())

		return
	}

	switch grant.kind {
	case "invitation":
		_, err = r.client.do("PUT", fmt.Sprintf("/users/invitations/%d", grant.id), map[string]any{"role": plan.Role.ValueString()})
	case "member":
		_, err = r.client.do("PUT", fmt.Sprintf("/users/%d", grant.id), map[string]any{"role": plan.Role.ValueString()})
	default:
		err = fmt.Errorf("no pending invitation or membership found for %s; the grant was revoked outside Terraform", plan.Email.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to update team member role", err.Error())

		return
	}

	grant, err = r.resolveGrant(plan.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read back the team member", err.Error())

		return
	}

	r.applyGrant(grant, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *teamMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	grant, err := r.resolveGrant(state.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve team member", err.Error())

		return
	}

	switch grant.kind {
	case "invitation":
		_, err = r.client.do("DELETE", fmt.Sprintf("/users/invitations/%d", grant.id), nil)
	case "member":
		_, err = r.client.do("DELETE", fmt.Sprintf("/users/%d", grant.id), nil)
	default:
		// Already revoked out of band; destroy is satisfied.
		return
	}
	if err != nil && !errors.Is(err, errNotFound) {
		resp.Diagnostics.AddError("Failed to revoke team member", err.Error())
	}
}

func (r *teamMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by email: terraform import sentinel_team_member.x person@example.com
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// resolveGrant finds the server-side state for an email: an active membership
// first (a stale invitation row must not shadow a real member), then a
// pending invitation. Emails compare case-insensitively.
func (r *teamMemberResource) resolveGrant(email string) (teamGrant, error) {
	members, err := r.client.do("GET", "/users", nil)
	if err != nil {
		return teamGrant{}, err
	}
	if row, ok := findByEmail(members, email); ok {
		role, _ := fieldString(row, "role")
		if role == "owner" {
			return teamGrant{}, fmt.Errorf("%s is the team owner; the owner's membership cannot be managed with sentinel_team_member", email)
		}
		id, _ := fieldInt(row, "id")
		status, _ := fieldString(row, "status")

		return teamGrant{kind: "member", id: id, role: role, status: status}, nil
	}

	invitations, err := r.client.do("GET", "/users/invitations", nil)
	if err != nil {
		return teamGrant{}, err
	}
	if row, ok := findByEmail(invitations, email); ok {
		id, _ := fieldInt(row, "id")
		role, _ := fieldString(row, "role")

		return teamGrant{kind: "invitation", id: id, role: role, status: "invited"}, nil
	}

	return teamGrant{}, nil
}

func findByEmail(listing map[string]any, email string) (map[string]any, bool) {
	rows, _ := listing["data"].([]any)
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if rowEmail, ok := fieldString(row, "email"); ok && strings.EqualFold(rowEmail, email) {
			return row, true
		}
	}

	return nil, false
}

func (r *teamMemberResource) applyGrant(grant teamGrant, model *teamMemberResourceModel) {
	model.ID = types.StringValue(model.Email.ValueString())
	model.Role = types.StringValue(grant.role)
	model.Status = types.StringValue(grant.status)

	if grant.kind == "member" {
		model.UserID = types.Int64Value(grant.id)
	} else {
		model.UserID = types.Int64Null()
	}
}
