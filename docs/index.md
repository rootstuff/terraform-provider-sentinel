---
page_title: "Sentinel Provider"
description: "Manage Sentinel uptime monitoring resources as code."
---

# Sentinel Provider

Manage [Sentinel](https://sentinel.rootstuff.io) uptime monitoring as code:
monitors and outbound webhook endpoints, through the versioned Sentinel API.

## Example Usage

```terraform
provider "sentinel" {
  # or set the SENTINEL_API_TOKEN environment variable
  api_token = var.sentinel_api_token
}

resource "sentinel_monitor" "storefront" {
  url           = "https://tickets.example.com"
  friendly_name = "Ticketing Storefront"
  check_types   = ["ssl", "dns", "domain"]
}
```

## Authentication

Create an API token on Sentinel's API settings page with `read` plus
whichever of `create`, `update`, and `delete` your resources need. Full API
access is included in the Pro and Business plans.

## Schema

- `api_token` (String, Sensitive) Sentinel API token. Falls back to the
  `SENTINEL_API_TOKEN` environment variable.
- `base_url` (String) API base URL. Defaults to
  `https://sentinel.rootstuff.io/api/v1`.
- `insecure_skip_verify` (Boolean) Skip TLS verification. Only for local
  development against a self-signed instance.
