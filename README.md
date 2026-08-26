# Terraform Provider for Sentinel

Manage [Sentinel](https://sentinel.rootstuff.io) uptime monitoring as code:
monitors, monitor groups, outbound webhook endpoints, and team
membership, through the versioned Sentinel API (`/api/v1`).

```hcl
terraform {
  required_providers {
    sentinel = {
      source = "rootstuff/sentinel"
    }
  }
}

provider "sentinel" {
  # or set SENTINEL_API_TOKEN
  api_token = var.sentinel_api_token
}

resource "sentinel_monitor" "storefront" {
  url           = "https://tickets.example.com"
  friendly_name = "Ticketing Storefront"
  check_types   = ["ssl", "dns", "domain"]
}

resource "sentinel_webhook_endpoint" "rootly" {
  name       = "Rootly"
  url        = var.rootly_webhook_url # write-only, carries its own secret
  auth_type  = "bearer"
  auth_token = var.rootly_token
  severities = ["critical", "warning"]
}
```

## Resources

- `sentinel_monitor`: http/ping/port monitors with ssl, dns, domain, keyword,
  json, and lighthouse sub-checks (plan gated by the account's subscription),
  including nested `keyword_settings` / `json_assertion_settings` blocks,
  HTTP auth (`auth_type`, `auth_username`, write-only `auth_password`), and
  `group_id` for group membership.
- `sentinel_group`: the dashboard's monitor groups (one level of nesting).
  Destroying a group ungroups its monitors, never deletes them.
- `sentinel_webhook_endpoint`: outbound alert destinations with bearer,
  basic, or custom-header auth. `url`, `auth_token`, and `signing_secret`
  are write-only: the API never returns them, so they are stored in state
  from configuration and drift on them is not detected.

The API token needs `read` plus whichever of `create`/`update`/`delete` the
managed resources require; full API access is a Pro or Business plan
feature.

## Development

```bash
go build -o terraform-provider-sentinel .
```

Point Terraform at the local build with a CLI config:

```hcl
provider_installation {
  dev_overrides {
    "rootstuff/sentinel" = "/path/to/terraform-provider-sentinel"
  }
  direct {}
}
```

`examples/main.tf` runs the full lifecycle against a local Sentinel instance
(`base_url` + `insecure_skip_verify` in the provider block).
