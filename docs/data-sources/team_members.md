---
page_title: "sentinel_team_members Data Source"
description: "The team's members with role, status, MFA state, and last sign-in: the access review."
---

# sentinel_team_members

The team's members, owner included, with the fields an access review needs:
role, active/disabled status, whether two-factor authentication is enabled,
and last sign-in. Read-only; manage membership with `sentinel_team_member`.

## Example Usage

```terraform
data "sentinel_team_members" "all" {}

# Fail the plan if anyone on the team has MFA off.
check "mfa_everywhere" {
  assert {
    condition = alltrue([
      for m in data.sentinel_team_members.all.members : m.mfa_enabled
    ])
    error_message = "A team member has two-factor authentication disabled."
  }
}
```

## Schema

### Read-Only

- `team` (String) Name of the team the token is scoped to.
- `members` (List of Object) One entry per member:
  - `id` (Number) User id.
  - `name` (String)
  - `email` (String)
  - `role` (String) `owner`, `admin`, `editor`, or `viewer`.
  - `status` (String) `active`, or `disabled` when the account is suspended.
  - `mfa_enabled` (Boolean) Whether two-factor authentication is confirmed.
  - `added_at` (String) When they joined this team (the team's creation date
    for the owner).
  - `last_login_at` (String) Most recent sign-in; null if never.
