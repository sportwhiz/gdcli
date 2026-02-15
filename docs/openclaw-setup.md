# OpenClaw Setup (CLI + Skill)

Installing `gdcli` gives you the executable, but OpenClaw skill behavior is configured separately.

## 1. Install gdcli

```bash
brew install sportwhiz/tap/gdcli
# or
go install github.com/sportwhiz/gdcli/cmd/gdcli@latest
```

## 2. Configure credentials in OpenClaw runtime

```bash
export GODADDY_API_KEY="..."
export GODADDY_API_SECRET="..."
```

Optional test-only override:

```bash
export GDCLI_BASE_URL=http://localhost:8787
```

## 3. Verify CLI availability

```bash
gdcli settings show --json
gdcli domains avail example.com --json
```

## 4. Add an OpenClaw skill (recommended)

Create a skill file at your skill path (for example: `$OPENCLAW_HOME/skills/gdcli/SKILL.md`) with clear guardrails.

Example `SKILL.md`:

```md
# gdcli skill

Use `gdcli` for GoDaddy domain investor workflows.

## Required defaults

- Always use `--json` or `--ndjson`.
- Log parsing must read `stdout` JSON envelope only.
- Treat non-zero exit codes as failures.

## Purchase safety policy

- Default path: `domains purchase <domain>` (tokenized dry-run/confirm).
- Never use `--auto` unless user explicitly requested auto-purchase mode.
- Before any purchase/renew action, check caps with `settings show --json`.

## Core command flow

1. `gdcli domains suggest "<query>" --json`
2. `gdcli domains avail-bulk <file> --ndjson`
3. `gdcli domains purchase <domain> --json`
4. If approved: `gdcli domains purchase <domain> --confirm <token> --json`

## Account and billing visibility

- Orders page: `gdcli account orders list --limit 50 --offset 0 --json`
- Subscriptions page: `gdcli account subscriptions list --limit 50 --offset 0 --json`
- Stream mode for agents: `gdcli account orders list --limit 50 --offset 0 --ndjson`

## DNS flow

- Audit: `gdcli dns audit --domains <file> --json`
- Apply dry-run first: `gdcli dns apply --template afternic-nameservers --domains <file> --dry-run --json`
```

## 5. Validate skill behavior in OpenClaw

Run a simple task and confirm it:

- calls `gdcli` commands,
- uses JSON mode,
- respects token-confirm purchase flow,
- does not auto-purchase unless explicitly instructed.
