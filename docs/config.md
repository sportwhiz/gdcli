# Configuration

## File location

- `~/.gdcli/config.json`

## Keys

- `api_environment`: `prod` or `ote`
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
