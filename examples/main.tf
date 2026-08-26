terraform {
  required_providers {
    sentinel = {
      source = "rootstuff/sentinel"
    }
  }
}

provider "sentinel" {
  base_url             = "https://uptime-watch.test/api/v1"
  insecure_skip_verify = true
}

resource "sentinel_monitor" "example" {
  url           = "https://example.net"
  friendly_name = "Terraform Example"
  check_types   = ["ssl", "domain"]
}

resource "sentinel_webhook_endpoint" "rootly" {
  name       = "Terraform Rootly"
  url        = "https://rootly.example.net/webhooks/incoming?secret=tf-test"
  auth_type  = "bearer"
  auth_token = "tf-test-token"
  severities = ["critical", "warning"]
}

resource "sentinel_team_member" "oncall" {
  email = "oncall@example.net"
  role  = "editor"
}

data "sentinel_team_members" "all" {}
