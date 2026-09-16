---
page_title: "sentinel_billing Resource"
description: "The account's billing recipients and the company details printed on invoices."
---

# sentinel_billing

The account's billing recipients and the company details printed on
invoices. Recipients receive every paid invoice as a PDF the moment Stripe
reports it paid, so an accounting inbox gets invoices without a login.
Billing information (company, address, tax ID) is pushed to the Stripe
customer and appears on invoices from the next one on.

There is exactly one billing record per account, so this resource is a
singleton: its `id` is always `billing`. It belongs to the account that
owns the plan; the token the provider uses must be that account's.

Destroying the resource clears the recipients and the billing information.
Nothing is deleted server-side.

## Example Usage

```terraform
resource "sentinel_billing" "account" {
  recipients = ["accounting@example.com", "ap@example.com"]

  company       = "Example Company LLC"
  address_line1 = "1 Main St"
  city          = "Sacramento"
  state         = "CA"
  postal_code   = "95814"
  country       = "US"

  tax_id_type = "us_ein"
  tax_id      = "12-3456789"
}
```

## Argument Reference

- `recipients` (List of String, Optional) Up to five email addresses that receive every paid invoice as a PDF. Stored lower-cased. Omit for none.
- `company` (String, Optional) Company or legal name invoices are made out to. Omit to invoice the account holder by name. Does not change the account's own name.
- `address_line1`, `address_line2`, `city`, `state`, `postal_code` (String, Optional) The billing address.
- `country` (String, Optional) Two-letter country code, for example `US` or `DE`.
- `tax_id_type` (String, Optional) Kind of tax ID: one of the keys `GET /api/v1/billing` lists under `tax_id_types`, for example `us_ein`, `eu_vat`, `gb_vat`. Required when `tax_id` is set.
- `tax_id` (String, Optional) The tax or VAT number.

The resource declares the whole record: an attribute left out of the
configuration is cleared on apply, not kept.

## Attribute Reference

- `id` (String) Always `billing`.

If Stripe rejects the billing information (an invalid tax ID, say), the
values are kept on the Sentinel account and the apply completes with a
warning that says why.

## Import

```shell
terraform import sentinel_billing.account billing
```
