# Quickstart

## Install

```bash
brew install sportwhiz/tap/gdcli
# or
go install github.com/sportwhiz/gdcli/cmd/gdcli@latest
```

## Initialize

```bash
export GDCLI_SHOPPER_ID="<YOUR_SHOPPER_ID>"
gdcli init --api-environment prod --resolve-customer-id --max-price 25 --max-daily-spend 100 --max-domains-per-day 5 --json
```

Optional macOS keychain bootstrap:

```bash
gdcli init --store-keychain --api-key "$GODADDY_API_KEY" --api-secret "$GODADDY_API_SECRET" --json
```

## Verify

```bash
gdcli init --verify --json
gdcli account identity show --json
gdcli settings show --json
gdcli domains avail example.com --json
```
