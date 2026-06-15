#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLS_BIN="$ROOT/.tools/bin"
LOCAL_NODE="$ROOT/.tools/node-v24.16.0-linux-x64/bin"
if ! command -v node >/dev/null 2>&1 && [[ ! -x "$LOCAL_NODE/node" ]]; then
  require_bootstrap_error=""
  command -v curl >/dev/null 2>&1 || require_bootstrap_error="curl"
  command -v sha256sum >/dev/null 2>&1 || require_bootstrap_error="${require_bootstrap_error:+$require_bootstrap_error, }sha256sum"
  command -v tar >/dev/null 2>&1 || require_bootstrap_error="${require_bootstrap_error:+$require_bootstrap_error, }tar"
  [[ -z "$require_bootstrap_error" ]] || { echo "Node bootstrap requires: $require_bootstrap_error" >&2; exit 1; }
  mkdir -p "$ROOT/.tools"
  archive="$ROOT/.tools/node-v24.16.0-linux-x64.tar.xz"
  curl -fsSL "https://nodejs.org/dist/v24.16.0/node-v24.16.0-linux-x64.tar.xz" -o "$archive"
  expected="$(curl -fsSL "https://nodejs.org/dist/v24.16.0/SHASUMS256.txt" | awk '/ node-v24.16.0-linux-x64.tar.xz$/ {print $1; exit}')"
  actual="$(sha256sum "$archive" | awk '{print $1}')"
  [[ -n "$expected" && "$actual" == "$expected" ]] || { echo "Node.js archive checksum mismatch." >&2; exit 1; }
  tar -xJf "$archive" -C "$ROOT/.tools"
  rm -f "$archive"
fi
if ! command -v node >/dev/null 2>&1 && [[ -x "$LOCAL_NODE/node" ]]; then
  export PATH="$LOCAL_NODE:$PATH"
fi

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command '$1' was not found on PATH." >&2
    exit 1
  fi
}

cd "$ROOT"

GO_MODULES=()
[[ -f go.mod ]] && GO_MODULES+=("$ROOT/go.mod")
while IFS= read -r -d '' module; do
  GO_MODULES+=("$module")
done < <(find core apps tools -path '*/node_modules' -prune -o -name go.mod -type f -print0 2>/dev/null)
if (( ${#GO_MODULES[@]} > 0 )); then
  require_command go
  GO_VERSION="$(go version)"
  if [[ ! "$GO_VERSION" =~ go1\.26\. ]]; then
    echo "Go 1.26.x is required; found $GO_VERSION" >&2
    exit 1
  fi
  echo "Go: $GO_VERSION"
  for module in "${GO_MODULES[@]}"; do
    module_dir="$(dirname "$module")"
    echo "Downloading Go modules in $module_dir"
    (cd "$module_dir" && go mod download)
  done
  mkdir -p "$TOOLS_BIN"
  if [[ ! -x "$TOOLS_BIN/wails" ]]; then
    echo "Installing pinned Wails CLI v2.12.0"
    GOBIN="$TOOLS_BIN" go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
  fi
  [[ "$($TOOLS_BIN/wails version 2>&1 | head -n 1)" =~ v2\.12\.0 ]] || {
    echo "Wails v2.12.0 is required." >&2
    exit 1
  }
fi

if [[ -f package-lock.json ]]; then
  require_command node
  require_command npm
  NODE_VERSION="$(node --version)"
  [[ "$NODE_VERSION" == "v24.16.0" ]] || { echo "Node.js 24.16.0 is required; found $NODE_VERSION" >&2; exit 1; }
  npm ci --prefix "$ROOT"
elif [[ -d apps ]]; then
  while IFS= read -r -d '' lockfile; do
    require_command node
    require_command npm
    NODE_VERSION="$(node --version)"
    [[ "$NODE_VERSION" == "v24.16.0" ]] || { echo "Node.js 24.16.0 is required; found $NODE_VERSION" >&2; exit 1; }
    package_dir="$(dirname "$lockfile")"
    echo "Installing locked web dependencies in $package_dir"
    npm ci --prefix "$package_dir"
  done < <(find apps -name package-lock.json -type f -print0)
fi

if [[ -f apps/android/gradlew ]]; then
  require_command java
  JAVA_VERSION="$(java -version 2>&1 | head -n 1)"
  if [[ ! "$JAVA_VERSION" =~ \"(17|18|19|20|21)\. ]]; then
    echo "JDK 17 through 21 is required; found $JAVA_VERSION" >&2
    exit 1
  fi
  echo "Java: $JAVA_VERSION"
  bash apps/android/gradlew --version
fi

(cd "$ROOT/tools" && GOWORK=off go run ./cmd/genfixtures -check)
echo "Setup complete. Run bash scripts/dev.sh check all"
