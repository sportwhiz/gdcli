# Architecture

## Layers

- `cmd/`: CLI routing and flag parsing
- `internal/services/`: business workflows
- `internal/godaddy/`: GoDaddy API client adapter
- `internal/rate/`: limiter + retry/backoff
- `internal/safety/`: confirmation token + auto-purchase checks
- `internal/budget/`: cap enforcement
- `internal/idempotency/`: operation keys and dedupe checks
- `internal/store/`: local state persistence
- `internal/output/`: JSON/NDJSON envelopes
- `internal/errors/`: typed app errors + exit code mapping

## Safety defaults

- Purchase requires token confirmation by default.
- Auto-purchase requires explicit enable + non-refund acknowledgment.
- Financial actions are cap-checked before execution.

## Determinism

- Machine output is envelope-based and parseable.
- Bulk mode can stream NDJSON for agent workflows.
