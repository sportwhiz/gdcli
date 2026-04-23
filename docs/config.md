# Configuration

## File location

- `~/.gdcli/config.json`

## Keys

- `api_environment`: `prod` or `ote`
- `shopper_id`: string (optional)
- `customer_id`: string (optional)
- `customer_id_resolved_at`: RFC3339 string (optional)
- `customer_id_source`: `manual` or `shopper_lookup`
- `auto_purchase_enabled`: bool
- `acknowledgment_hash`: string
- `max_price_per_domain`: number (USD)
- `max_daily_spend`: number (USD)
- `max_domains_per_day`: integer
- `default_years`: integer
- `default_dns_template`: string
- `output_default`: `json`

## State files

In `~/.gdcli/`:

- `operations.jsonl`: idempotency + spend ledger
- `confirm_tokens.json`: purchase confirmation tokens

## Environment identity overrides

- `GDCLI_SHOPPER_ID`: if set, overrides `shopper_id` in runtime config
- `GDCLI_CUSTOMER_ID`: if set, overrides `customer_id` in runtime config

## Consent recording

Domain registration requires the caller to accept one or more registrar
agreements (e.g. the `DNRA` Domain Name Registration Agreement for `.com`).
GoDaddy records *who* accepted the agreements in the `agreedBy` field — it is
the legal audit trail for consent.

- `GDCLI_CONSENT_AGREED_BY`: string to record as the registrant identifier,
  typically the IP address of the human who clicked "I agree" in your UI.
  When unset, gdcli detects the local outbound IP; if detection fails, falls
  back to the literal string `"gdcli"` and prints a warning on stderr. For
  production automation, set this explicitly to the end-user's IP so the
  consent record reflects reality.
