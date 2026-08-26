---
page_title: "sentinel_team_member Resource"
description: "An access grant to the team, keyed by email: invite, role, and removal as code."
---

# sentinel_team_member

An access grant to the team, keyed by email. Creating the resource sends a
Sentinel invitation email; the person becomes a member when they accept.
Both states satisfy the resource, so an accepted invitation is not drift:
`status` simply moves from `invited` to `active`.

Changing `role` updates in place on either side of that transition (a
pending invitation is re-roled without a second email). Destroying the
resource cancels the pending invitation or removes the member, whichever
exists; the person's Sentinel account itself is never deleted.

The team owner cannot be managed with this resource, and the plan's seat
limit applies to invitations exactly as it does in the dashboard.

## Example Usage

```terraform
resource "sentinel_team_member" "oncall" {
  email = "oncall@example.com"
  role  = "editor"
}

resource "sentinel_team_member" "security" {
  email = "security@example.com"
  role  = "viewer"
}
```

## Schema

### Required

- `email` (String) Email address the grant is for. Changing it replaces the
  resource: the old grant is revoked and the new address is invited.
- `role` (String) Team role: `admin`, `editor`, or `viewer`.

### Read-Only

- `id` (String) Same as `email`.
- `status` (String) `invited` while the invitation is pending, then `active`
  (or `disabled` if the account is suspended) once accepted.
- `user_id` (Number) The member's user id once the invitation is accepted;
  null while pending.

## Import

Import by email; it resolves whether the grant is a pending invitation or an
accepted membership:

```shell
terraform import sentinel_team_member.oncall oncall@example.com
```
