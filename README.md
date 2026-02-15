# GDCLI CLI

Agent-first GoDaddy domain investor CLI with strict purchase safety defaults, budget guardrails, deterministic JSON/NDJSON output, and local testability via a mock registrar API.

## Install

### Homebrew (after first release)

```bash
brew install sportwhiz/tap/gdcli
```

### Go install

```bash
go install github.com/sportwhiz/gdcli/cmd/gdcli@latest
```

If needed, add Go bin to `PATH`:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

Verify:

```bash
gdcli help --json
```

## What is implemented

- Discovery: `domains suggest`, `domains avail`, `domains avail-bulk`
- Acquisition: dry-run + confirmation token purchase flow, optional `--auto`
- Renewals: `domains renew`, `domains renew-bulk`
- Portfolio: `domains list` with filters
- DNS: `dns audit`, `dns apply` (built-in + custom JSON templates)
- Settings: `settings auto-purchase enable|disable`, `settings caps set`, `settings show`
- Safety: non-refundable ack phrase required for auto purchase
- Budget controls: max price/domain, max daily spend, max domains/day
- Idempotency ledger and confirmation token store in `~/.gdcli/`
- Exit codes: `0,2,3,4,5,6,7,8,9`

## Credentials

Production/OTE usage:

- `GODADDY_API_KEY`
- `GODADDY_API_SECRET`

macOS fallback keychain entries (service `gdcli`):

- account `godaddy_api_key`
- account `godaddy_api_secret`

## Output contract

- Structured payloads on `stdout`
- Logs/warnings on `stderr`
- Global flags:
  - `--json` (default)
  - `--ndjson`
  - `--quiet`

## Fast local test (no real GoDaddy calls)

1. Start mock API:

```bash
go run ./cmd/mock-godaddy
```

2. In another shell, set test env:

```bash
export GDCLI_BASE_URL=http://localhost:8787
export GODADDY_API_KEY=dummy
export GODADDY_API_SECRET=dummy
```

3. Run sample flows:

```bash
go run . settings show --json
go run . domains suggest "garlic bread" --limit 3 --json
go run . domains avail example.com --json
printf "example.com\nnewbrand.ai\n" > /tmp/domains.txt
go run . domains avail-bulk /tmp/domains.txt --concurrency 2 --ndjson

go run . domains purchase example.com --json
# copy confirmation_token from output
# go run . domains purchase example.com --confirm <TOKEN> --json

go run . settings auto-purchase enable --ack "I UNDERSTAND PURCHASES ARE FINAL" --json
go run . domains purchase newbrand.ai --auto --json

go run . domains list --tld com --json
printf "alpha.com\nbrand.ai\n" > /tmp/portfolio.txt
go run . dns audit --domains /tmp/portfolio.txt --json
go run . dns apply --template afternic-nameservers --domains /tmp/portfolio.txt --dry-run --json
```

## Custom DNS template format

Use a JSON file with either/both keys:

```json
{
  "nameservers": ["ns1.afternic.com", "ns2.afternic.com"],
  "records": [
    {"type": "A", "name": "@", "data": "52.71.57.184", "ttl": 600}
  ]
}
```

Then apply:

```bash
go run . dns apply --template /path/to/template.json --domains /tmp/portfolio.txt --json
```

## Build and verify

```bash
go test ./...
go run . --help
```

## Maintainer release setup (GitHub + Homebrew tap)

This repo includes:

- GoReleaser config: `.goreleaser.yaml`
- GitHub release workflow: `.github/workflows/release.yml`

One-time setup:

1. Create tap repo: `https://github.com/sportwhiz/homebrew-tap`
2. Add repo secret `HOMEBREW_TAP_GITHUB_TOKEN` in `sportwhiz/gdcli` (token needs `repo` scope to push formula updates to `sportwhiz/homebrew-tap`).
3. Push a semver tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

On tag push, GitHub Actions will:

- run tests
- build binaries for macOS/Linux/Windows
- create GitHub Release artifacts + checksums
- update Homebrew formula in `sportwhiz/homebrew-tap/Formula/gdcli.rb`
