# Output and Exit Codes

## Output streams

- `stdout`: structured payloads only
- `stderr`: warnings/logs

## Modes

- `--json`: single envelope
- `--ndjson`: one envelope per record

Envelope fields:

- `command`
- `timestamp_utc`
- `request_id`
- `result` or `error`

Error fields:

- `code`
- `message`
- `details`
- `retryable`
- `doc_url`

## Exit codes

- `0`: success
- `2`: validation/usage error
- `3`: auth error
- `4`: rate-limit exhausted
- `5`: provider/internal error
- `6`: budget violation
- `7`: confirmation error
- `8`: safety policy violation
- `9`: partial failure
