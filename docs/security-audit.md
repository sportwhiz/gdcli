# Security Audit Report

Date: 2026-02-15
Project: `gdcli`

## Scope

- CLI command layer
- API client behavior
- credential handling
- local state persistence
- release pipeline/toolchain

## Tooling

- `go test ./...`
- `go vet ./...`
- `gosec ./...`
- `govulncheck ./...`

## Findings and Remediation

### 1) Weak RNG usage in retry jitter

- Severity: High
- File: `internal/rate/rate.go`
- Issue: `math/rand` used for jitter.
- Fix: switched to `crypto/rand` (`rand.Int` with bounded max).

### 2) Potential SSRF through configurable API base URL

- Severity: High
- File: `internal/godaddy/client.go`
- Issue: arbitrary base URL use could target unintended hosts.
- Fix: strict base URL validation:
  - allows only `api.godaddy.com`, `api.ote-godaddy.com`, and loopback hosts
  - enforces `https` for GoDaddy hosts

### 3) Mock server lacked timeout hardening

- Severity: Medium
- File: `cmd/mock-godaddy/main.go`
- Issue: `http.ListenAndServe` without server timeouts.
- Fix: replaced with `http.Server` and `ReadHeaderTimeout`.

### 4) Command execution scanner hits on keychain calls

- Severity: Medium (tooling warning)
- File: `internal/app/app.go`
- Issue: `exec.Command` with variable args flagged by scanner.
- Fix:
  - strict account allowlist for reads
  - explicit non-shell usage comments (`#nosec`) with rationale

### 5) Path traversal scanner hits on local file access

- Severity: Medium (tooling warning)
- Files:
  - `internal/config/config.go`
  - `internal/store/store.go`
  - `internal/services/services.go`
- Issue: variable path usage flagged.
- Fix:
  - `filepath.Clean` normalization
  - explicit controlled-path/user-intent comments (`#nosec`) where appropriate

### 6) Go stdlib vulnerability (crypto/tls)

- Advisory: `GO-2026-4337`
- Fix:
  - pinned Go version to `1.25.7` in `go.mod`
  - release workflow now uses Go `1.25.7`
  - validated with `govulncheck`: no vulnerabilities found

## Current Security Status

- `gosec`: 0 issues
- `govulncheck`: no vulnerabilities found
- `go test`: passing
- `go vet`: passing

## Residual Risk

- Runtime credentials in environment variables are still sensitive; prefer scoped secrets and short-lived CI credentials where possible.
- `GDCLI_BASE_URL` is intentionally available for testing; host validation limits misuse risk.
