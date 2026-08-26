package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

/*
sentinel_team_members is the read-only access review: every member of the
token's team (owner included) with role, status, MFA state, and last sign-in.
Useful for wiring access checks into CI ("fail the pipeline if anyone has MFA
off") or feeding an audit report, without managing membership.
*/

func NewTeamMembersDataSource() datasource.DataSource {
	return &teamMembersDataSource{}
}

type teamMembersDataSource struct {
	client *apiClient
}

type teamMembersDataSourceModel struct {
	Team    types.String      `tfsdk:"team"`
	Members []teamMemberModel `tfsdk:"members"`
}

type teamMemberModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Email       types.String `tfsdk:"email"`
	Role        types.String `tfsdk:"role"`
	Status      types.String `tfsdk:"status"`
	MfaEnabled  types.Bool   `tfsdk:"mfa_enabled"`
	AddedAt     types.String `tfsdk:"added_at"`
	LastLoginAt types.String `tfsdk:"last_login_at"`
}

func (d *teamMembersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_members"
}

func (d *teamMembersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The team's members (owner included) with the fields an access review needs: role, active/disabled status, MFA state, and last sign-in.",
		Attributes: map[string]schema.Attribute{
			"team": schema.StringAttribute{
				MarkdownDescription: "Name of the team the token is scoped to.",
				Computed:            true,
			},
			"members": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"email": schema.StringAttribute{
							Computed: true,
						},
						"role": schema.StringAttribute{
							MarkdownDescription: "`owner`, `admin`, `editor`, or `viewer`.",
							Computed:            true,
						},
						"status": schema.StringAttribute{
							MarkdownDescription: "`active`, or `disabled` when the account is suspended.",
							Computed:            true,
						},
						"mfa_enabled": schema.BoolAttribute{
							MarkdownDescription: "Whether two-factor authentication is confirmed on the account.",
							Computed:            true,
						},
						"added_at": schema.StringAttribute{
							MarkdownDescription: "When they joined this team (the team's creation date for the owner).",
							Computed:            true,
						},
						"last_login_at": schema.StringAttribute{
							MarkdownDescription: "Most recent sign-in; null if never.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *teamMembersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*apiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *apiClient, got %T", req.ProviderData))

		return
	}

	d.client = client
}

func (d *teamMembersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	listing, err := d.client.do("GET", "/users", nil)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list team members", err.Error())

		return
	}

	var state teamMembersDataSourceModel
	if team, ok := fieldString(listing, "team"); ok {
		state.Team = types.StringValue(team)
	}

	rows, _ := listing["data"].([]any)
	state.Members = make([]teamMemberModel, 0, len(rows))
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		member := teamMemberModel{
			Name:        types.StringNull(),
			Email:       types.StringNull(),
			Role:        types.StringNull(),
			Status:      types.StringNull(),
			AddedAt:     types.StringNull(),
			LastLoginAt: types.StringNull(),
		}
		if id, ok := fieldInt(row, "id"); ok {
			member.ID = types.Int64Value(id)
		}
		if name, ok := fieldString(row, "name"); ok {
			member.Name = types.StringValue(name)
		}
		if email, ok := fieldString(row, "email"); ok {
			member.Email = types.StringValue(email)
		}
		if role, ok := fieldString(row, "role"); ok {
			member.Role = types.StringValue(role)
		}
		if status, ok := fieldString(row, "status"); ok {
			member.Status = types.StringValue(status)
		}
		if mfa, ok := fieldBool(row, "mfa_enabled"); ok {
			member.MfaEnabled = types.BoolValue(mfa)
		}
		if addedAt, ok := fieldString(row, "added_at"); ok {
			member.AddedAt = types.StringValue(addedAt)
		}
		if lastLogin, ok := fieldString(row, "last_login_at"); ok {
			member.LastLoginAt = types.StringValue(lastLogin)
		}

		state.Members = append(state.Members, member)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
