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

- `monitor_type` (String) `http`, `ping`, `port`, `heartbeat`, or `cron`.
  Defaults to `http`. Changing it replaces the monitor. Heartbeat/cron
  monitors receive inbound pings (see `ping_url`) instead of being probed;
  they take no `url`.
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

- `http_method` (String) GET (default), POST, PUT, PATCH, DELETE, HEAD, OPTIONS.
- `request_headers` (Map of String) Extra request headers for http checks.
- `request_body` (String) Body for POST/PUT/PATCH checks.
- `accepted_status_codes` (Set of String) Codes counted as up, exact (`"200"`)
  or classes (`"2xx"`). Defaults to 2xx and 3xx.
- `slow_response_threshold` (Number) Alert when responses exceed this many
  milliseconds.
- `heartbeat_interval` (Number) Expected seconds between pings. Required for
  `heartbeat` monitors.
- `heartbeat_cron_expression` (String) Cron schedule for pings. Required for
  `cron` monitors.
- `heartbeat_timezone` (String) Timezone for the cron schedule.
- `heartbeat_grace` (Number) Grace seconds after a missed ping before alerting.

### Read-Only

- `id` (String) Monitor id.
- `status` (String) Current status as last evaluated.
- `ping_url` (String, Sensitive) The inbound ping URL for heartbeat/cron
  monitors. Treat it as a secret: anyone holding it can fake health.

## Heartbeat example

The dead-man pattern: a server pings only while healthy, and silence alerts.
`ping_url` wires straight into whatever does the pinging.

```terraform
resource "sentinel_monitor" "db_replication" {
  monitor_type       = "heartbeat"
  friendly_name      = "standby replication"
  heartbeat_interval = 60
  heartbeat_grace    = 120
}

output "replication_ping_url" {
  value     = sentinel_monitor.db_replication.ping_url
  sensitive = true
}
```

## Import

```shell
terraform import sentinel_monitor.storefront 190
```
