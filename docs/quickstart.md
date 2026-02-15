# Quickstart

## Install

```bash
brew install sportwhiz/tap/gdcli
# or
go install github.com/sportwhiz/gdcli/cmd/gdcli@latest
```

## Initialize

```bash
gdcli init --api-environment prod --max-price 25 --max-daily-spend 100 --max-domains-per-day 5 --json
```

Optional macOS keychain bootstrap:

```bash
gdcli init --store-keychain --api-key "$GODADDY_API_KEY" --api-secret "$GODADDY_API_SECRET" --json
```

## Verify

```bash
gdcli init --verify --json
gdcli settings show --json
gdcli domains avail example.com --json
```
