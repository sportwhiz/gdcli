# gdcli

[![Release](https://img.shields.io/github/v/release/sportwhiz/gdcli)](https://github.com/sportwhiz/gdcli/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/sportwhiz/gdcli/release.yml?branch=main)](https://github.com/sportwhiz/gdcli/actions)
[![Homebrew Tap](https://img.shields.io/badge/homebrew-sportwhiz%2Ftap-blue)](https://github.com/sportwhiz/homebrew-tap)

GoDaddy-focused, agent-friendly domain investor CLI.

`gdcli` is JSON-first, safe-by-default for purchases, and built for automation tools like OpenClaw.

## Documentation

- [`docs/quickstart.md`](docs/quickstart.md)
- [`docs/commands.md`](docs/commands.md)
- [`docs/config.md`](docs/config.md)
- [`docs/output.md`](docs/output.md)
- [`docs/architecture.md`](docs/architecture.md)
- [`docs/openclaw-setup.md`](docs/openclaw-setup.md)

## Install

### Homebrew

```bash
brew install sportwhiz/tap/gdcli
```

### Go Install

```bash
go install github.com/sportwhiz/gdcli/cmd/gdcli@latest
```

If needed:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

Verify:

```bash
gdcli help --json
```

Check installed version and update status:

```bash
gdcli version --check --json
```

Startup commands also perform a best-effort update notice check (cached to once per 24h) and print notices to `stderr` only.

If you are using OpenClaw, also complete skill setup:

- [`docs/openclaw-setup.md`](docs/openclaw-setup.md)

## 5-Minute Setup

1. Set credentials.

```bash
export GODADDY_API_KEY="your_key"
export GODADDY_API_SECRET="your_secret"
export GDCLI_SHOPPER_ID="<YOUR_SHOPPER_ID>"
```

Use placeholders in shared docs/snippets. Do not publish real shopper IDs.

2. Verify config and auth.

```bash
gdcli settings show --json
gdcli domains avail example.com --json
```

3. Safe purchase flow (default).

```bash
gdcli domains purchase example.com --json
# copy confirmation_token
gdcli domains purchase example.com --confirm <TOKEN> --json
```

Bootstrap with guided init:

```bash
gdcli init --api-environment prod --resolve-customer-id --max-price 25 --max-daily-spend 100 --max-domains-per-day 5 --verify --json
```

Confirm identity was stored for v2 customer-scoped calls:

```bash
gdcli account identity show --json
```

## Safety Model

- Purchases default to dry-run style with confirmation token.
- Auto-purchase is disabled by default.
- Auto-purchase requires explicit non-refund acknowledgment.
- Budget caps are enforced before financial operations.

Enable auto mode:

```bash
gdcli settings auto-purchase enable --ack "I UNDERSTAND PURCHASES ARE FINAL" --json
```

## Output Model

- `stdout`: structured JSON/NDJSON only.
- `stderr`: logs and warnings.
- Global flags:
  - `--json` (default)
  - `--ndjson`
  - `--quiet`

## Upgrading

```bash
brew update && brew upgrade gdcli
```

```bash
go install github.com/sportwhiz/gdcli/cmd/gdcli@latest
```

Built-in helper:

```bash
gdcli self-update --json
gdcli version --check --json
```

## Common Workflows

### Discovery

```bash
gdcli domains suggest "garlic bread" --limit 10 --json
gdcli domains avail garlicbread.com --json
```

### Bulk Availability

```bash
printf "alpha.com\nbeta.ai\ngamma.net\n" > /tmp/domains.txt
gdcli domains avail-bulk /tmp/domains.txt --concurrency 5 --ndjson
```

### Renewals

```bash
gdcli domains renew alpha.com --years 1 --dry-run --json
gdcli domains renew alpha.com --years 1 --auto-approve --json
```

### Portfolio Filter

```bash
gdcli domains list --expiring-in 30 --tld com --json
gdcli domains list --expiring-in 30 --tld com --with-nameservers --concurrency 5 --json
```

### Account Intelligence

```bash
gdcli account orders list --limit 5 --offset 0 --json
gdcli account subscriptions list --limit 5 --offset 0 --json
gdcli account orders list --limit 5 --offset 0 --ndjson
```

### DNS Audit and Apply

```bash
printf "alpha.com\nbrand.ai\n" > /tmp/portfolio.txt
gdcli dns audit --domains /tmp/portfolio.txt --json
gdcli dns apply --template afternic-nameservers --domains /tmp/portfolio.txt --dry-run --json
```

## Commands

### Top-level

- `version [--check]`
- `self-update`

### `domains`

- `domains suggest <query> [--tlds com,ai] [--limit N]`
- `domains avail <domain>`
- `domains avail-bulk <file> [--concurrency N]`
- `domains purchase <domain> [--confirm TOKEN] [--auto] [--years N]`
- `domains renew <domain> --years N [--dry-run] [--auto-approve]`
- `domains renew-bulk <file> --years N [--dry-run] [--auto-approve]`
- `domains list [--expiring-in N] [--tld TLD] [--contains TEXT] [--with-nameservers] [--concurrency N]`
- `domains portfolio [--expiring-in N] [--tld TLD] [--contains TEXT] [--concurrency N]` (agent-friendly full list with nameservers)
- `domains detail <domain> [--includes actions,contacts,dnssecRecords,registryStatusCodes]`
- `domains actions <domain> [--type ACTION_TYPE]`
- `domains change-of-registrant <domain>`
- `domains usage <yyyymm>`
- `domains maintenances [--id MAINTENANCE_ID]`
- `domains notifications next|optin list|optin set|schema|ack`
- `domains contacts set <domain> --body-json '<json>' [--apply]`
- `domains nameservers set <domain> --nameservers ns1,ns2 [--apply]`
- `domains dnssec add <domain> --body-json '<json>' [--apply]`
- `domains forwarding get|create|update <fqdn> [--body-json '<json>'] [--apply]`
- `domains privacy-forwarding get|set <domain> [--body-json '<json>'] [--apply]`
- `domains auth-code regenerate <domain> [--apply]`
- `domains register schema|validate|purchase ...`
- `domains transfer status|validate|start|in-accept|in-cancel|in-restart|in-retry|out|out-accept|out-reject ...`
- `domains redeem <domain> [--body-json '<json>'] [--apply]`

### `account`

- `account orders list [--limit N] [--offset N]`
- `account subscriptions list [--limit N] [--offset N]`
- `account identity show`
- `account identity set --shopper-id ID [--customer-id ID]`
- `account identity resolve`

### `dns`

- `dns audit --domains <file>`
- `dns apply --template <afternic-nameservers|parking|template.json> --domains <file> [--dry-run]`

### `settings`

- `settings auto-purchase enable --ack "I UNDERSTAND PURCHASES ARE FINAL"`
- `settings auto-purchase disable`
- `settings caps set --max-price USD --max-daily-spend USD --max-domains-per-day N`
- `settings show`

## Configuration

Config file: `~/.gdcli/config.json`

| Key | Default | Purpose |
|---|---:|---|
| `api_environment` | `prod` | GoDaddy environment (`prod` or `ote`) |
| `shopper_id` | empty | Shopper id used to resolve/store customer id |
| `customer_id` | empty | Customer id used for v2 customer-scoped API calls |
| `customer_id_resolved_at` | empty | RFC3339 timestamp of last successful shopper->customer resolution |
| `customer_id_source` | empty | `manual` or `shopper_lookup` |
| `auto_purchase_enabled` | `false` | Allows `domains purchase --auto` |
| `acknowledgment_hash` | empty | Non-refund acknowledgement marker |
| `max_price_per_domain` | `25` | Per-domain purchase cap (USD) |
| `max_daily_spend` | `100` | Daily spend cap (USD) |
| `max_domains_per_day` | `5` | Daily domain count cap |
| `default_years` | `1` | Default registration/renew years |
| `default_dns_template` | `afternic-nameservers` | Default DNS template |
| `output_default` | `json` | Default output mode |

Writes under v2 command groups are safe-by-default: `--apply` is required for execution; without it commands return dry-run intent payloads.

## Environment Variables

- `GODADDY_API_KEY`
- `GODADDY_API_SECRET`
- `GDCLI_SHOPPER_ID` (optional; used for customer-id resolution)
- `GDCLI_CUSTOMER_ID` (optional; overrides stored customer_id)
- `GDCLI_BASE_URL` (optional API override for testing)
- `GDCLI_DISABLE_UPDATE_CHECK` (`1`/`true`/`yes` to disable startup update notices)

macOS keychain fallback is supported under service `gdcli` with accounts:

- `godaddy_api_key`
- `godaddy_api_secret`

## Local Mock Testing (No Real Registrar Calls)

1. Start mock server.

```bash
go run ./cmd/mock-godaddy
```

2. In another shell.

```bash
export GDCLI_BASE_URL=http://localhost:8787
export GODADDY_API_KEY=dummy
export GODADDY_API_SECRET=dummy
```

3. Run commands.

```bash
gdcli domains avail example.com --json
gdcli domains purchase example.com --json
```

## Custom DNS Template

Template JSON supports either or both keys.

```json
{
  "nameservers": ["ns1.afternic.com", "ns2.afternic.com"],
  "records": [
    {"type": "A", "name": "@", "data": "52.71.57.184", "ttl": 600}
  ]
}
```

Apply:

```bash
gdcli dns apply --template /path/to/template.json --domains /tmp/portfolio.txt --json
```

## Exit Codes

- `0`: success
- `2`: validation/usage error
- `3`: auth/credentials error
- `4`: provider rate limit exhausted
- `5`: provider/internal error
- `6`: budget violation
- `7`: confirmation token error
- `8`: safety policy violation
- `9`: partial failure

## Troubleshooting

### `auth_error` / missing credentials

Set both `GODADDY_API_KEY` and `GODADDY_API_SECRET`, then re-run.

### Budget violations (`exit 6`)

Increase caps with:

```bash
gdcli settings caps set --max-price 50 --max-daily-spend 500 --max-domains-per-day 20 --json
```

### Auto-purchase blocked (`exit 8`)

Enable with exact phrase:

```bash
gdcli settings auto-purchase enable --ack "I UNDERSTAND PURCHASES ARE FINAL" --json
```

### Bulk command partial failures (`exit 9`)

Use `--ndjson` and inspect per-line errors for domains that failed.

## Development

```bash
go test ./...
go run ./cmd/gdcli help --json
```

## Release Maintainer Notes

Release config lives in `.goreleaser.yaml` and `.github/workflows/release.yml`.

Required setup:

1. Repo: `sportwhiz/gdcli`
2. Tap repo: `sportwhiz/homebrew-tap`
3. Secret in `sportwhiz/gdcli`: `HOMEBREW_TAP_GITHUB_TOKEN`

Release:

```bash
git tag v0.1.3
git push origin v0.1.3
```

This publishes binaries/checksums to GitHub Releases and updates `Formula/gdcli.rb` in the tap repo.
