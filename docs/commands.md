# Command Reference

## Top-level

- `gdcli init`
- `gdcli version [--check]`
- `gdcli self-update`
- `gdcli domains ...`
- `gdcli account ...`
- `gdcli dns ...`
- `gdcli settings ...`

## Init

- `gdcli init --api-environment prod|ote`
- `gdcli init --max-price N --max-daily-spend N --max-domains-per-day N`
- `gdcli init --shopper-id ID [--resolve-customer-id]`
- `gdcli init --enable-auto-purchase --ack "I UNDERSTAND PURCHASES ARE FINAL"`
- `gdcli init --store-keychain --api-key KEY --api-secret SECRET` (macOS)
- `gdcli init --verify`

## Domains

- `gdcli domains suggest <query> [--tlds com,ai] [--limit N]`
- `gdcli domains avail <domain>`
- `gdcli domains avail-bulk <file> [--concurrency N]`
- `gdcli domains purchase <domain> [--years N]`
- `gdcli domains purchase <domain> --confirm TOKEN [--years N]`
- `gdcli domains purchase <domain> --auto [--years N]`
- `gdcli domains renew <domain> --years N [--dry-run] [--auto-approve]`
- `gdcli domains renew-bulk <file> --years N [--dry-run] [--auto-approve]`
- `gdcli domains list [--expiring-in N] [--tld TLD] [--contains TEXT]`
- `gdcli domains portfolio [--expiring-in N] [--tld TLD] [--contains TEXT] [--concurrency N]`
- `gdcli domains detail <domain> [--includes actions,contacts,dnssecRecords,registryStatusCodes]`
- `gdcli domains actions <domain> [--type ACTION_TYPE]`
- `gdcli domains change-of-registrant <domain>`
- `gdcli domains usage <yyyymm>`
- `gdcli domains maintenances [--id MAINTENANCE_ID]`
- `gdcli domains notifications next`
- `gdcli domains notifications optin list`
- `gdcli domains notifications optin set --types TYPE_A,TYPE_B [--apply]`
- `gdcli domains notifications schema <type>`
- `gdcli domains notifications ack <notificationId> [--apply]`
- `gdcli domains contacts set <domain> --body-json '<json>' [--apply]`
- `gdcli domains nameservers set <domain> --nameservers ns1,ns2 [--apply]`
- `gdcli domains dnssec add <domain> --body-json '<json>' [--apply]`
- `gdcli domains forwarding get|create|update <fqdn> [--body-json '<json>'] [--apply]`
- `gdcli domains privacy-forwarding get|set <domain> [--body-json '<json>'] [--apply]`
- `gdcli domains auth-code regenerate <domain> [--apply]`
- `gdcli domains register schema <tld>`
- `gdcli domains register validate|purchase --body-json '<json>' [--apply]`
- `gdcli domains transfer status|validate|start|in-accept|in-cancel|in-restart|in-retry|out|out-accept|out-reject <domain> [--body-json '<json>'] [--apply]`
- `gdcli domains redeem <domain> [--body-json '<json>'] [--apply]`

## DNS

- `gdcli dns audit --domains <file>`
- `gdcli dns apply --template afternic-nameservers --domains <file> [--dry-run]`
- `gdcli dns apply --template parking --domains <file> [--dry-run]`
- `gdcli dns apply --template /path/template.json --domains <file> [--dry-run]`

## Account

- `gdcli account orders list [--limit N] [--offset N]`
- `gdcli account subscriptions list [--limit N] [--offset N]`
- `gdcli account identity show`
- `gdcli account identity set --shopper-id ID [--customer-id ID]`
- `gdcli account identity resolve`

## Settings

- `gdcli settings auto-purchase enable --ack "I UNDERSTAND PURCHASES ARE FINAL"`
- `gdcli settings auto-purchase disable`
- `gdcli settings caps set --max-price N --max-daily-spend N --max-domains-per-day N`
- `gdcli settings show`

## Update Behavior

- Normal commands may print update notices to `stderr` (cached every 24 hours).
- Use `--quiet` or set `GDCLI_DISABLE_UPDATE_CHECK=1` to suppress startup notices.
- Explicit update commands remain:
  - `gdcli version --check --json`
  - `gdcli self-update --json`
