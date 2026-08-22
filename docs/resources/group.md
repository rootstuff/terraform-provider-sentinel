---
page_title: "sentinel_group Resource"
description: "A monitor group: the organizational unit the dashboard's grouped view is built from."
---

# sentinel_group

A monitor group: the organizational unit the dashboard's grouped view is
built from. Monitors join a group through the `group_id` attribute on
`sentinel_monitor`. Nesting is one level deep: a top-level group and its
subgroups.

Destroying a group ungroups its monitors and promotes its subgroups to the
top level. It never deletes monitors.

## Example Usage

```terraform
resource "sentinel_group" "clients" {
  name        = "Agency Clients"
  description = "Everything billed monthly"
}

resource "sentinel_group" "aurora" {
  name      = "Aurora Tickets"
  parent_id = sentinel_group.clients.id
}

resource "sentinel_monitor" "storefront" {
  url      = "https://tickets.example.com"
  group_id = sentinel_group.aurora.id
}
```

## Schema

### Required

- `name` (String) Group name, unique within the team, max 60 characters.

### Optional

- `description` (String) Optional description, max 255 characters.
- `parent_id` (Number) Parent group id for a subgroup. The parent must be a
  top-level group, and a group that has subgroups cannot itself be nested.

### Read-Only

- `id` (String) Group identifier.

## Import

```shell
terraform import sentinel_group.clients 42
```
