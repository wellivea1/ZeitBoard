#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ACTION="${1:-check}"
COMPONENT="${2:-all}"
LOCAL_WAILS="$ROOT/.tools/bin/wails"
LOCAL_NODE="$ROOT/.tools/node-v24.16.0-linux-x64/bin"
if ! command -v node >/dev/null 2>&1 && [[ -x "$LOCAL_NODE/node" ]]; then
  export PATH="$LOCAL_NODE:$PATH"
fi

case "$ACTION" in
  check|test|build|dev|fixtures) ;;
  *) echo "Unknown action: $ACTION" >&2; exit 2 ;;
esac

case "$COMPONENT" in
  all|contracts|core|desktop|trusted-web|android) ;;
  *) echo "Unknown component: $COMPONENT" >&2; exit 2 ;;
esac

run_contracts() {
  if [[ "$ACTION" == fixtures ]]; then
    (cd "$ROOT/tools" && GOWORK=off go run ./cmd/genfixtures)
  else
    (cd "$ROOT/tools" && GOWORK=off go run ./cmd/genfixtures -check)
    "$ROOT/scripts/validate-contracts.sh"
  fi
}

run_core() {
  modules=()
  [[ -f "$ROOT/go.mod" ]] && modules+=("$ROOT/go.mod")
  while IFS= read -r -d '' module; do
    modules+=("$module")
  done < <(find "$ROOT/core" "$ROOT/apps" -path '*/node_modules' -prune -o -name go.mod -type f -print0 2>/dev/null)
  if (( ${#modules[@]} == 0 )); then
    echo "Skipping core: no Go modules are present."
    return
  fi
  cd "$ROOT"
  case "$ACTION" in
    check)
      unformatted="$(find core apps/desktop -name '*.go' -type f -print0 2>/dev/null | xargs -0 -r gofmt -l)"
      [[ -z "$unformatted" ]] || { echo "gofmt required:" >&2; echo "$unformatted" >&2; exit 1; }
      for module in "${modules[@]}"; do
        (cd "$(dirname "$module")" && go test ./... && go vet ./...)
      done
      ;;
    test)
      for module in "${modules[@]}"; do
        (cd "$(dirname "$module")" && go test ./...)
      done
      ;;
    dev) (cd "$ROOT/core" && go test ./...) ;;
    build)
      for module in "${modules[@]}"; do
        (cd "$(dirname "$module")" && go build ./...)
      done
      ;;
  esac
}

has_npm_script() {
  node -e 'const p=require(process.argv[1]); process.exit(p.scripts && p.scripts[process.argv[2]] ? 0 : 1)' "$1/package.json" "$2"
}

run_web() {
  local path="$1"
  local label="$2"
  if [[ ! -f "$path/package.json" ]]; then
    echo "Skipping $label: package.json is not present at $path."
    return
  fi
  local script="$ACTION"
  [[ "$ACTION" == check ]] && script=build
  if ! has_npm_script "$path" "$script"; then
    if [[ "$script" == test ]]; then
      echo "Skipping $label tests: no test script is defined."
      return
    fi
    echo "$label does not define an npm '$script' script." >&2
    exit 1
  fi
  npm --prefix "$path" run "$script"
}

run_desktop() {
  local desktop="$ROOT/apps/desktop"
  local wails=""
  if [[ -x "$LOCAL_WAILS" ]]; then
    wails="$LOCAL_WAILS"
  elif command -v wails >/dev/null 2>&1; then
    wails="$(command -v wails)"
  fi
  if [[ "$ACTION" == dev && -f "$desktop/wails.json" && -n "$wails" ]]; then
    (cd "$desktop" && "$wails" dev)
    return
  fi
  if [[ "$ACTION" == build && -f "$desktop/wails.json" ]]; then
    [[ -n "$wails" ]] || { echo "Wails CLI not found. Run scripts/setup.sh first." >&2; exit 1; }
    (cd "$desktop" && "$wails" build -nosyncgomod)
    return
  fi
  run_web "$desktop/frontend" "desktop frontend"
}

run_android() {
  local android="$ROOT/apps/android"
  if [[ ! -f "$android/gradlew" ]]; then
    echo "Skipping Android: Gradle wrapper is not present."
    return
  fi
  local task
  case "$ACTION" in
    check) task=check ;;
    test) task=testDebugUnitTest ;;
    build) task=assembleDebug ;;
    dev) task=installDebug ;;
    fixtures) return ;;
  esac
  (cd "$android" && bash ./gradlew --no-daemon "$task")
}

if [[ "$ACTION" == fixtures ]]; then
  run_contracts
  exit 0
fi

if [[ "$ACTION" == dev && "$COMPONENT" == all ]]; then
  echo "Choose one component for a long-running dev command." >&2
  exit 2
fi

if [[ "$COMPONENT" == all ]]; then
  COMPONENTS=(contracts core desktop trusted-web android)
else
  COMPONENTS=("$COMPONENT")
fi

for item in "${COMPONENTS[@]}"; do
  case "$item" in
    contracts) run_contracts ;;
    core) run_core ;;
    desktop) run_desktop ;;
    trusted-web) run_web "$ROOT/apps/trusted-web-prototype" "trusted web" ;;
    android) run_android ;;
  esac
done
