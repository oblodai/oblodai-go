#!/usr/bin/env bash
# Fail when zz_generated_*.go are not what the generator makes of the gateway's contract.
#
# Regenerates into a temporary directory with the backend's tools/sdkgen (from
# services/core/api/openapi.json, checked against names.lock) and compares file by file. The
# backend checkout is $OBLODAI_BACKEND, else ../oblodai-backend next to this repository. Without a
# backend that has tools/sdkgen the check is skipped, loudly — unless OBLODAI_BACKEND is set or
# --require is passed, when it fails instead. Fix drift by regenerating (`make sdk` in the
# backend), never by hand.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
backend="${OBLODAI_BACKEND:-$root/../oblodai-backend}"
sdkgen="$backend/tools/sdkgen"
spec="$backend/services/core/api/openapi.json"

if [ ! -d "$sdkgen/cmd/sdkgen" ] || [ ! -f "$spec" ]; then
  message="no generator at $sdkgen (set OBLODAI_BACKEND to the backend checkout)"
  if [ "${1:-}" = "--require" ] || [ -n "${OBLODAI_BACKEND:-}" ]; then
    echo "check-generated: $message" >&2
    exit 1
  fi
  echo "  (skipped: $message)"
  exit 0
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
# The generator is its own module with its own toolchain; the SDK's GOTOOLCHAIN does not apply.
if ! (cd "$sdkgen" && GOTOOLCHAIN="${SDKGEN_GOTOOLCHAIN:-go1.26.6}" GOFLAGS= GOWORK=off \
  go run ./cmd/sdkgen -spec "$spec" -lang go -out "$tmp" -lock "$root/names.lock") >"$tmp.log" 2>&1; then
  echo "check-generated: sdkgen failed:" >&2
  cat "$tmp.log" >&2
  rm -f "$tmp.log"
  exit 1
fi
rm -f "$tmp.log"

stale=()
for f in "$tmp"/zz_generated_*.go "$root"/zz_generated_*.go; do
  name="$(basename "$f")"
  if ! cmp -s "$tmp/$name" "$root/$name"; then
    stale+=("$name")
  fi
done
if [ "${#stale[@]}" -gt 0 ]; then
  printf 'check-generated: stale %s; regenerate with `make sdk` in the backend\n' \
    "$(printf '%s\n' "${stale[@]}" | sort -u | paste -sd, -)" >&2
  exit 1
fi
echo "generated code matches $spec"
