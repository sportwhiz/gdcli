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

## DNS

- `gdcli dns audit --domains <file>`
- `gdcli dns apply --template afternic-nameservers --domains <file> [--dry-run]`
- `gdcli dns apply --template parking --domains <file> [--dry-run]`
- `gdcli dns apply --template /path/template.json --domains <file> [--dry-run]`

## Account

- `gdcli account orders list [--limit N] [--offset N]`
- `gdcli account subscriptions list [--limit N] [--offset N]`

## Settings

- `gdcli settings auto-purchase enable --ack "I UNDERSTAND PURCHASES ARE FINAL"`
- `gdcli settings auto-purchase disable`
- `gdcli settings caps set --max-price N --max-daily-spend N --max-domains-per-day N`
- `gdcli settings show`
