#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Validation lives in the Go tools module: it checks every contract schema
# against the draft 2020-12 metaschema and validates each fixture and the
# sample configuration against the schemas.
# tools is an isolated module (not in go.work); GOWORK=off builds it standalone.
cd "$ROOT/tools"
GOWORK=off go run ./cmd/validatecontracts
