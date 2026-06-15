#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VALIDATOR="$ROOT/.tools/venv/bin/check-jsonschema"
cd "$ROOT"

if [[ ! -x "$VALIDATOR" ]]; then
  if command -v check-jsonschema >/dev/null 2>&1; then
    VALIDATOR="$(command -v check-jsonschema)"
  else
    echo "check-jsonschema 0.37.3 is required. Run ./scripts/setup.sh first." >&2
    exit 1
  fi
fi

validate() {
  "$VALIDATOR" --schemafile "$ROOT/contracts/v1/$1" "$2"
}

"$VALIDATOR" --check-metaschema "$ROOT"/contracts/v1/*.schema.json

validate observation-set.schema.json "$ROOT/testdata/v1/observations.json"
validate correction-set.schema.json "$ROOT/testdata/v1/corrections.json"
validate phase-estimate.schema.json "$ROOT/testdata/v1/phase-estimate.json"
validate schedule-request.schema.json "$ROOT/testdata/v1/schedule-request.json"
validate schedule-proposals.schema.json "$ROOT/testdata/v1/schedule-proposals.json"
validate share-profile.schema.json "$ROOT/testdata/v1/share-profile-default-deny.json"
validate share-profile.schema.json "$ROOT/testdata/v1/share-profile-allowlisted.json"
validate trusted-view.schema.json "$ROOT/testdata/v1/trusted-view-default-deny.json"
validate trusted-view.schema.json "$ROOT/testdata/v1/trusted-view.json"
validate config.schema.json "$ROOT/config.example.json"

echo "Validated all v1 fixtures and sample configuration."
