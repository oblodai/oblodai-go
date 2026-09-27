#!/usr/bin/env bash
# Vendor the parts of the backend's contract the SDK's tests read, so CI runs the shared
# conformance suite without a checkout of the (private) backend:
#
#   contract/services/core/api/openapi.json   the spec's x-oblodai-signing only (signing and
#                                             webhook vectors, header names, limits)
#   contract/tools/sdkgen/conformance/*.json  the shared behaviour suite
#
# The layout mirrors the backend, so the suite's relative "spec" path resolves unchanged.
#
#   scripts/sync-contract.sh           rewrite the snapshot from $OBLODAI_BACKEND (else ../oblodai-backend)
#   scripts/sync-contract.sh --check   fail when the snapshot differs from the backend (run by `make drift`)
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
backend="${OBLODAI_BACKEND:-$root/../oblodai-backend}"
spec="$backend/services/core/api/openapi.json"
suite="$backend/tools/sdkgen/conformance"
if [ ! -f "$spec" ] || [ ! -d "$suite" ]; then
  echo "sync-contract: no contract at $backend (set OBLODAI_BACKEND)" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/services/core/api" "$tmp/tools/sdkgen/conformance"
python3 - "$spec" "$tmp/services/core/api/openapi.json" <<'PY'
import json, sys
spec = json.load(open(sys.argv[1], encoding="utf-8"))
with open(sys.argv[2], "w", encoding="utf-8") as out:
    json.dump({"x-oblodai-signing": spec["x-oblodai-signing"]}, out, indent=1, ensure_ascii=False, sort_keys=True)
    out.write("\n")
PY
cp "$suite"/*.json "$tmp/tools/sdkgen/conformance/"

if [ "${1:-}" = "--check" ]; then
  if ! diff -r "$tmp" "$root/contract" >/dev/null 2>&1; then
    echo "sync-contract: contract/ is stale against $backend; run scripts/sync-contract.sh" >&2
    exit 1
  fi
  echo "contract snapshot matches $backend"
  exit 0
fi
rm -rf "$root/contract"
mkdir -p "$root/contract"
cp -r "$tmp"/. "$root/contract/"
echo "contract/ refreshed from $backend"
