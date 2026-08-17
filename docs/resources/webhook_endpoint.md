---
page_title: "sentinel_webhook_endpoint Resource"
description: "An outbound webhook destination for team alerts."
---

# sentinel_webhook_endpoint

An outbound webhook destination for team alerts: an incident tool, an
internal automation, a partner intake.

`url`, `auth_token`, and `signing_secret` are write-only: the API never
returns them (webhook URLs routinely carry credentials), so they are stored
in state from configuration and drift on them is not detected. After a
`terraform import`, the first apply resends them from configuration.

## Example Usage

```terraform
resource "sentinel_webhook_endpoint" "rootly" {
  name       = "Rootly"
  url        = var.rootly_webhook_url
  auth_type  = "bearer"
  auth_token = var.rootly_token
  severities = ["critical", "warning"]
}
```

## Schema

### Required

- `name` (String) Display name, max 100 characters.
- `url` (String, Sensitive) HTTPS destination URL. Write-only.

### Optional

- `auth_type` (String) `none`, `bearer`, `basic`, or `header`. Defaults to
  `none`.
- `auth_token` (String, Sensitive) Credential for bearer/basic/header auth.
  Write-only.
- `auth_header_name` (String) Header carrying the token when `auth_type` is
  `header`.
- `signing_secret` (String, Sensitive) HMAC signing secret, 16 to 255
  characters. Write-only.
- `severities` (Set of String) Severities delivered here: `critical`,
  `warning`, `info`. Defaults to all three.
- `is_active` (Boolean) Inactive endpoints are kept but receive nothing.
  Defaults to `true`.

### Read-Only

- `id` (String) Endpoint id.
- `url_host` (String) Host part of the configured URL, as reported by the
  API.

## Import

```shell
terraform import sentinel_webhook_endpoint.rootly 2
```
