---
page_title: "sentinel_monitor Resource"
description: "An uptime monitor checked from every monitored region."
---

# sentinel_monitor

An uptime monitor: an HTTP(S) URL, ping host, or TCP port checked from every
monitored region, with optional sub-checks.

## Example Usage

```terraform
resource "sentinel_monitor" "storefront" {
  url                     = "https://tickets.example.com"
  friendly_name           = "Ticketing Storefront"
  check_interval          = 1
  check_types             = ["ssl", "dns", "domain"]
  domain_expiry_threshold = 30
}
```

## Schema

### Required

- `url` (String) The URL (http monitors) or host (ping/port monitors) to check.

### Optional

- `monitor_type` (String) `http`, `ping`, or `port`. Defaults to `http`.
  Changing it replaces the monitor.
- `port` (Number) TCP port. Required when `monitor_type` is `port`.
- `friendly_name` (String) Display name in the dashboard and alerts.
- `check_interval` (Number) Minutes between checks, 0.5 to 60 (plan limits
  apply). Defaults to the plan's interval.
- `check_types` (Set of String) Sub-checks: `ssl`, `dns`, `domain`,
  `keyword`, `json`, `lighthouse` (plan gated).
- `monitored_regions` (Set of String) Regions to check from. Defaults to all
  active regions.
- `ssl_expiry_threshold` (Number) Days before SSL expiry to alert.
- `domain_expiry_threshold` (Number) Days before domain expiry to alert.
  Domain expiry is tracked at the registrable domain (a subdomain monitor
  tracks its parent's registration), and one alert fires per team per
  domain regardless of how many monitors share it.
- `request_timeout` (Number) Per-check timeout in seconds, 1 to 60.
- `follow_redirects` (Boolean) Follow HTTP redirects.
- `error_text_detection` (Boolean) Detect server error text (PHP and
  framework error output) on otherwise-successful pages.

### Read-Only

- `id` (String) Monitor id.
- `status` (String) Current status as last evaluated.

## Import

```shell
terraform import sentinel_monitor.storefront 190
```
