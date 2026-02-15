# Security Best Practices Audit Report

Date: 2026-02-15  
Repository: /Users/cookie/domain-os  
Scope: Full codebase audit (Go CLI + mock server + update checker)

## Executive Summary

The codebase is in good shape overall: tests, race tests, and vet are all clean, and the previously discussed hardening changes are present (token store locking, token pruning, bounded provider response reads, mock body limits, safer mock bind default, and operation-log warning behavior).

I found **2 actionable security findings** and **1 scanner false positive**:

1. **Medium**: unbounded update-check response body decode in `internal/update/checker.go`.
2. **Low/Medium**: potential portability/overflow concern flagged on `uintptr -> int` cast in file-locking calls.
3. **False positive**: SSRF warning on update check; target URL is compile-time constant GitHub API endpoint.

No critical or high-severity exploitable issue was found in current local repo state.

## Validation Performed

- `go test ./...` (pass)
- `go test -race ./...` (pass)
- `go vet ./...` (pass)
- `gosec ./...` (3 findings; triaged below)
- Manual review of: credentials handling, outbound HTTP controls, token confirmation flow, file I/O, mock server exposure/limits, and idempotency paths.

## Findings

### Medium

#### SEC-001: Unbounded response body in update checker
- File: `internal/update/checker.go:77`
- Evidence: `json.NewDecoder(resp.Body).Decode(&payload)` is called without `io.LimitReader` or equivalent cap.
- Impact: A very large response body (from remote endpoint/proxy/MITM failure mode) could increase memory pressure or fail unpredictably.
- Recommendation:
  - Apply bounded read for update endpoint responses (for example `io.LimitReader(resp.Body, 1<<20)`).
  - Keep timeout as-is and return explicit decode-size errors.

### Low / Medium

#### SEC-002: `uintptr -> int` cast in flock calls
- File: `internal/store/store.go:155`, `internal/store/store.go:158`
- Evidence: `syscall.Flock(int(f.Fd()), ...)`
- Impact: On platforms where `int` width is smaller than pointer width, static analyzers flag a potential overflow/truncation.
- Practical risk: Low on common 64-bit macOS/Linux environments, but worth hardening for portability and scanner hygiene.
- Recommendation:
  - Guard conversion with explicit bounds check before cast, or use a helper that validates FD conversion.

## False Positives / Accepted

### FP-001: SSRF warning in update checker
- File: `internal/update/checker.go:63`
- Scanner: `gosec` G704 taint-based SSRF
- Triage: The request target is compile-time constant `https://api.github.com/repos/sportwhiz/gdcli/releases/latest` at `internal/update/checker.go:12`; user input is not used to build URL.
- Conclusion: Not a real SSRF vector in current implementation.

## Confirmed Good Security Controls

- Token consume path uses atomic load-mutate-save with file lock:
  - `internal/store/store.go:143`
  - `internal/safety/safety.go:77`
- Token pruning removes expired/used records:
  - `internal/safety/safety.go:59`
- Provider response body limits enforced by endpoint class:
  - `internal/godaddy/client.go:42`
  - `internal/godaddy/client.go:633`
  - `internal/godaddy/client.go:641`
- Mock server default bind is localhost + body size limits:
  - `cmd/mock-godaddy/main.go:171`
  - `cmd/mock-godaddy/main.go:178`
- Operation log write failures are surfaced as warnings:
  - `internal/services/services.go:82`

## Residual Risks / Notes

- `RequireAutoEnabled` still checks only `ackHash != ""` (`internal/safety/safety.go:110`), by design per current scope.
- Output intentionally keeps `SetEscapeHTML(false)` for CLI JSON readability (`internal/output/output.go:38`, `internal/output/output.go:44`).
- `operations.jsonl` has no rotation policy yet; operational concern, low direct security impact.

## Recommended Next Fix Order

1. Add update-check body cap (`SEC-001`).
2. Harden flock FD conversion (`SEC-002`).
3. Optionally revisit strict auto-purchase hash equality if you want stronger local safety invariants.
